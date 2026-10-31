// Package server contains HTTP handlers, middleware and routes.
package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
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
	AllowSignup  bool
	APIToken     string // optional bearer token of the JSON API
}

// Server is the HTTP application.
type Server struct {
	cfg     Config
	svc     *service.Service
	view    *view.Renderer
	static  fs.FS
	gzipped map[string][]byte // compressed text assets by URL path
	limiter *loginLimiter
	started time.Time
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
	if err := svc.LoadRules(context.Background()); err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	gzipped, err := gzipAssets(static)
	if err != nil {
		return nil, fmt.Errorf("compress assets: %w", err)
	}
	return &Server{cfg: cfg, svc: svc, view: renderer, static: static, gzipped: gzipped, limiter: newLoginLimiter(), started: time.Now()}, nil
}

// gzipAssets compresses the text assets once at startup. Pages stay uncompressed:
// they hold the CSRF token next to reflected input, which would allow BREACH.
func gzipAssets(static fs.FS) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(static, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch path.Ext(name) {
		case ".css", ".js", ".svg":
		default:
			return nil
		}
		body, err := fs.ReadFile(static, name)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(body); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		out["/static/"+name] = buf.Bytes()
		return nil
	})
	return out, err
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

	admin := func(pattern string, h handlerFunc) { handle(pattern, adminOnly(h)) }
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
	handle("GET /signup", s.signupForm)
	handle("POST /signup", s.signup)
	handle("GET /invite/{token}", s.inviteForm)
	handle("POST /invite/{token}", s.inviteAccept)

	handle("GET /{$}", s.today)
	handle("GET /onboarding", s.onboarding)
	handle("POST /onboarding", s.onboardingFinish)
	handle("POST /onboarding/skip", s.onboardingSkip)
	handle("GET /templates", s.templateList)
	handle("GET /templates/suggest", s.templateSuggest)
	handle("GET /habits", s.habitList)
	handle("POST /habits/delete", s.habitDeleteAll)
	handle("GET /habits/new", s.habitNew)
	handle("POST /habits/new", s.habitSave)
	handle("GET /habits/{id}", s.habitDetail)
	handle("GET /habits/{id}/edit", s.habitEdit)
	handle("POST /habits/{id}/edit", s.habitSave)
	handle("POST /habits/{id}/check", s.habitCheck)
	handle("POST /habits/{id}/add", s.habitAdd)
	handle("POST /habits/{id}/undo", s.habitUndo)
	handle("POST /habits/{id}/delete", s.habitDelete)
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
	handle("POST /rewards/{id}/free", s.rewardFree)
	handle("POST /rewards/{id}/duplicate", s.rewardDuplicate)
	handle("POST /rewards/{id}/pause", s.rewardPause)
	handle("POST /rewards/{id}/delete", s.rewardDelete)
	handle("POST /rewards/redemptions/{id}/undo", s.redemptionUndo)
	handle("POST /reward-ideas/{key}", s.rewardIdeaAdd)
	handle("POST /joker/buy", s.jokerBuy)

	handle("GET /celebrate", s.celebrate)
	handle("POST /celebrate", s.celebrateNext)

	handle("GET /quest", s.questDetail)
	handle("GET /profile", s.profile)
	handle("GET /settings/areas", s.areas)
	handle("POST /settings/areas", s.areaSave)
	handle("POST /settings/areas/{id}/delete", s.areaDelete)
	handle("GET /settings/reminders", s.reminders)
	handle("POST /settings/reminders", s.remindersSave)
	handle("GET /settings/joker", s.jokerRules)
	handle("POST /settings/joker", s.jokerRulesSave)
	handle("GET /settings/figure", s.figures)
	handle("POST /settings/figure", s.figureSave)
	handle("GET /settings/quests", s.questHabits)
	handle("POST /settings/quests/{quest}/habits/{id}", s.questHabitSave)
	handle("GET /settings/backup", s.backup)
	handle("GET /settings/server", s.serverInfo)
	handle("POST /settings/password", s.passwordChange)
	handle("POST /settings/holidays", s.holidaysSave)
	handle("GET /export", s.exportData)
	handle("POST /import", s.importData)
	handle("POST /settings/reset", s.settingsReset)
	handle("POST /push/subscribe", s.pushSubscribe)
	handle("POST /push/unsubscribe", s.pushUnsubscribe)

	admin("GET /admin", s.adminHome)
	admin("GET /admin/levels", s.adminLevels)
	admin("POST /admin/levels", s.adminLevelsSave)
	admin("POST /admin/levels/reset", s.adminLevelsReset)
	admin("POST /admin/levels/{level}/formula", s.adminLevelFormula)
	admin("GET /admin/badges", s.adminBadges)
	admin("POST /admin/badges/feature", s.adminBadgesFeature)
	admin("GET /admin/badges/new", s.adminBadgeNew)
	admin("POST /admin/badges/new", s.adminBadgeSave)
	admin("GET /admin/badges/{key}/edit", s.adminBadgeEdit)
	admin("POST /admin/badges/{key}/edit", s.adminBadgeSave)
	admin("POST /admin/badges/{key}/active", s.adminBadgeActive)
	admin("POST /admin/badges/{key}/delete", s.adminBadgeDelete)
	admin("GET /admin/badges/categories", s.adminCategories)
	admin("POST /admin/badges/categories", s.adminCategorySave)
	admin("POST /admin/badges/categories/{key}/delete", s.adminCategoryDelete)
	admin("GET /admin/badges/transfer", s.adminBadgesTransfer)
	admin("GET /admin/badges/export", s.adminBadgesExport)
	admin("POST /admin/badges/import", s.adminBadgesImport)
	admin("GET /admin/users", s.adminUsers)
	admin("POST /admin/users", s.adminUserCreate)
	admin("POST /admin/users/{id}/role", s.adminUserRole)
	admin("POST /admin/users/{id}/disabled", s.adminUserDisabled)
	admin("POST /admin/users/{id}/password", s.adminUserPassword)
	admin("POST /admin/users/{id}/delete", s.adminUserDelete)
	admin("POST /admin/invites", s.adminInviteCreate)
	admin("POST /admin/invites/{token}/delete", s.adminInviteDelete)
	admin("GET /admin/system", s.adminSystem)
	admin("GET /admin/system/backup", s.adminBackup)
	admin("GET /admin/quests", s.adminQuests)
	admin("POST /admin/quests/settings", s.adminQuestsSettings)
	admin("GET /admin/quests/new", s.adminQuestNew)
	admin("POST /admin/quests/new", s.adminQuestSave)
	admin("GET /admin/quests/{id}/edit", s.adminQuestEdit)
	admin("POST /admin/quests/{id}/edit", s.adminQuestSave)
	admin("POST /admin/quests/{id}/active", s.adminQuestActive)
	admin("POST /admin/quests/{id}/delete", s.adminQuestDelete)
	admin("GET /admin/quests/plan", s.adminQuestPlan)
	admin("POST /admin/quests/plan", s.adminPlanChange)
	admin("POST /admin/quests/rules", s.adminPlanRules)
	admin("POST /admin/quests/events", s.adminEventAdd)
	admin("POST /admin/quests/events/{id}/delete", s.adminEventDelete)
	admin("GET /admin/areas", s.adminPlaceholder("Bereiche", "areas", "Die Standardwerte der Bereiche sind vorerst fest. Jeder Benutzer verwaltet seine eigene Kopie im Profil."))
	admin("GET /admin/templates", s.adminPlaceholder("Vorlagen", "templates", "Die Vorlagen sind vorerst fest eingebaut."))
	admin("GET /admin/economy", s.adminPlaceholder("Taler & Joker", "economy", "Preise und Grenzen sind vorerst fest eingebaut."))

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
	case errors.Is(err, service.ErrLastAdmin):
		return http.StatusConflict, "Der letzte aktive Admin muss erhalten bleiben"
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
		h := w.Header()
		// The stylesheet cannot version the font URL, and embedded files have no
		// validators: a short lifetime would refetch it and shift the text.
		if r.URL.Query().Has("v") || strings.HasPrefix(r.URL.Path, "/static/fonts/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "public, max-age=3600")
		}
		body, ok := s.gzipped[r.URL.Path]
		if !ok {
			files.ServeHTTP(w, r)
			return
		}
		h.Set("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			files.ServeHTTP(w, r)
			return
		}
		h.Set("Content-Type", mime.TypeByExtension(path.Ext(r.URL.Path)))
		h.Set("Content-Encoding", "gzip")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
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

// isHTMX reports a request for a fragment. Boosted links and forms and history
// restores also come from HTMX, but they want the complete page.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" &&
		r.Header.Get("HX-History-Restore-Request") != "true"
}

// redirect sends the browser elsewhere, also from inside an HTMX request.
func redirect(w http.ResponseWriter, r *http.Request, target string) error {
	if isHTMX(r) {
		w.Header().Set("HX-Location", target)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
	return nil
}

func (s *Server) page(r *http.Request, title, tab string, data any) view.Page {
	return view.Page{Title: title, Tab: tab, CSRF: csrfFrom(r), Admin: currentUser(r).IsAdmin(), NoBadges: s.svc.Rules().BadgesOff, Data: data}
}
