// Package server contains HTTP handlers, middleware and routes.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

// Config holds the runtime options.
type Config struct {
	Version      string
	Runtime      string // shown on the server page, e.g. "Docker"
	CookieSecure bool
	APIToken     string // optional bearer token of the JSON API
}

// Server is the HTTP application.
type Server struct {
	cfg     Config
	svc     *service.Service
	view    *view.Renderer
	static  fs.FS
	limiter *loginLimiter
}

// New wires up the server. files must contain templates/ and static/.
func New(cfg Config, svc *service.Service, files fs.FS) (*Server, error) {
	renderer, err := view.New(files)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	return &Server{cfg: cfg, svc: svc, view: renderer, static: static, limiter: newLoginLimiter()}, nil
}

// Handler returns the complete handler chain.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	protection := http.NewCrossOriginProtection()
	return logging(securityHeaders(protection.Handler(s.authenticate(mux))))
}

func (s *Server) routes(mux *http.ServeMux) {
	handle := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, s.wrap(h))
	}

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.Handle("GET /static/", s.staticFiles())
	handle("GET /sw.js", s.serviceWorker)
	handle("GET /manifest.webmanifest", s.manifest)
	handle("GET /offline", s.offline)

	handle("GET /setup", s.setupForm)
	handle("POST /setup", s.setup)
	handle("GET /login", s.loginForm)
	handle("POST /login", s.login)
	handle("POST /logout", s.logout)

	handle("GET /{$}", s.today)
	handle("GET /habits", s.habitList)
	handle("GET /habits/new", s.habitNew)
	handle("POST /habits/new", s.habitSave)
	handle("GET /habits/{id}", s.habitDetail)
	handle("GET /habits/{id}/edit", s.habitEdit)
	handle("POST /habits/{id}/edit", s.habitSave)
	handle("POST /habits/{id}/check", s.habitCheck)
	handle("POST /habits/{id}/relapse", s.habitRelapse)
	handle("POST /habits/{id}/joker", s.habitJoker)
	handle("POST /habits/{id}/archive", s.habitArchive)
	handle("POST /habits/{id}/restore", s.habitRestore)
	handle("POST /habits/{id}/move", s.habitMove)

	handle("GET /badges", s.badges)

	handle("GET /rewards", s.rewards)
	handle("GET /rewards/new", s.rewardNew)
	handle("POST /rewards/new", s.rewardSave)
	handle("GET /rewards/{id}/edit", s.rewardEdit)
	handle("POST /rewards/{id}/edit", s.rewardSave)
	handle("POST /rewards/{id}/redeem", s.rewardRedeem)
	handle("POST /rewards/{id}/archive", s.rewardArchive)
	handle("POST /joker/buy", s.jokerBuy)

	handle("GET /celebrate", s.celebrate)
	handle("POST /celebrate", s.celebrateNext)

	handle("GET /profile", s.profile)
	handle("GET /settings/areas", s.areas)
	handle("POST /settings/areas", s.areaSave)
	handle("POST /settings/areas/{id}/delete", s.areaDelete)
	handle("GET /settings/reminders", s.reminders)
	handle("POST /settings/reminders", s.remindersSave)
	handle("GET /settings/joker", s.jokerRules)
	handle("POST /settings/joker", s.jokerRulesSave)
	handle("GET /settings/backup", s.backup)
	handle("GET /settings/server", s.serverInfo)
	handle("POST /settings/password", s.passwordChange)
	handle("POST /settings/holidays", s.holidaysSave)
	handle("GET /export", s.exportData)
	handle("POST /import", s.importData)
	handle("POST /push/subscribe", s.pushSubscribe)
	handle("POST /push/unsubscribe", s.pushUnsubscribe)

	handle("GET /api/v1/habits", s.apiHabits)
	handle("GET /api/v1/checkins", s.apiCheckins)
	handle("POST /api/v1/habits/{id}/checkins", s.apiCheck)
	handle("GET /api/v1/stats", s.apiStats)
}

// httpError carries a status to the client.
type httpError struct {
	status  int
	message string
}

func (e httpError) Error() string { return e.message }

func badRequest(message string) error { return httpError{http.StatusBadRequest, message} }

// wrap turns handler errors into responses in one place.
func (s *Server) wrap(h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.svc.Settle(r.Context()); err != nil {
			slog.Error("settle failed", "err", err)
		}
		err := h(w, r)
		if err == nil {
			return
		}
		status, message := classify(err)
		if status >= 500 {
			slog.Error("request failed", "path", r.URL.Path, "err", err)
		}
		http.Error(w, message, status)
	})
}

func classify(err error) (int, string) {
	var he httpError
	switch {
	case errors.As(err, &he):
		return he.status, he.message
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound, "Nicht gefunden"
	case errors.Is(err, service.ErrOutOfRange):
		return http.StatusBadRequest, "Nur die letzten 7 Tage können bearbeitet werden"
	case errors.Is(err, service.ErrInsufficient):
		return http.StatusConflict, "Nicht genug Taler"
	case errors.Is(err, service.ErrLocked):
		return http.StatusConflict, "Noch nicht freigeschaltet"
	case errors.Is(err, service.ErrJokerLimit):
		return http.StatusConflict, "Joker-Grenze erreicht"
	case errors.Is(err, service.ErrNotAllowed):
		return http.StatusConflict, "Nicht möglich"
	case errors.Is(err, context.Canceled):
		return 499, "Abgebrochen"
	default:
		return http.StatusInternalServerError, "Interner Fehler"
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Store.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) staticFiles() http.Handler {
	files := http.StripPrefix("/static/", http.FileServerFS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self'; " +
		"form-action 'self'; base-uri 'none'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		// Not no-referrer: browsers would then send "Origin: null" on form posts,
		// which the cross-origin check rejects on plain HTTP outside localhost.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store") // static assets override this
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			return
		}
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, httpError{http.StatusNotFound, "Nicht gefunden"}
	}
	return id, nil
}

// requestDay reads the optional date parameter, today by default.
func (s *Server) requestDay(r *http.Request) (model.Day, error) {
	raw := r.FormValue("date")
	if raw == "" {
		return s.svc.Today(), nil
	}
	d, err := model.ParseDay(raw)
	if err != nil {
		return 0, badRequest("Ungültiges Datum")
	}
	return d, nil
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// redirect sends the browser elsewhere, also from inside an HTMX request.
func redirect(w http.ResponseWriter, r *http.Request, target string) error {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
	return nil
}

func (s *Server) page(r *http.Request, title, tab string, data any) view.Page {
	return view.Page{Title: title, Tab: tab, CSRF: csrfFrom(r), Data: data}
}
