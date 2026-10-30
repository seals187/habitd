package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/service"
)

const (
	sessionCookie   = "habitd_session"
	sessionLifetime = 30 * 24 * time.Hour
	minPasswordLen  = 8
	maxLoginFails   = 5
	loginLockout    = 15 * time.Minute
)

type ctxKey int

const csrfKey ctxKey = 1

func csrfFrom(r *http.Request) string {
	v, _ := r.Context().Value(csrfKey).(string)
	return v
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b)
}

// publicPath lists what works without a session.
func publicPath(p string) bool {
	switch p {
	case "/healthz", "/login", "/setup", "/sw.js", "/manifest.webmanifest", "/offline":
		return true
	}
	return strings.HasPrefix(p, "/static/")
}

func (s *Server) passwordSet(ctx context.Context) (bool, error) {
	hash, err := s.svc.Store.Q(ctx).Setting(service.KeyPasswordHash, "")
	return hash != "", err
}

// authenticate enforces login and CSRF tokens.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if s.validBearer(r) {
			next.ServeHTTP(w, r)
			return
		}
		csrf, ok := s.session(r)
		if !ok {
			s.rejectAnonymous(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf")
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(csrf)) != 1 {
				http.Error(w, "CSRF-Token ungültig", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey, csrf)))
	})
}

func (s *Server) validBearer(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && s.cfg.APIToken != "" && strings.HasPrefix(r.URL.Path, "/api/") &&
		subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.APIToken)) == 1
}

func (s *Server) session(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	csrf, err := s.svc.Store.Q(r.Context()).Session(hashToken(c.Value), time.Now())
	return csrf, err == nil
}

func (s *Server) rejectAnonymous(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	target := "/login"
	if set, err := s.passwordSet(r.Context()); err == nil && !set {
		target = "/setup"
	}
	_ = redirect(w, r, target)
}

func (s *Server) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) error {
	token := randomToken()
	expires := time.Now().Add(sessionLifetime)
	if err := s.svc.Store.Q(r.Context()).CreateSession(hashToken(token), randomToken(), expires); err != nil {
		return err
	}
	s.setCookie(w, token, int(sessionLifetime.Seconds()))
	return nil
}

type authForm struct {
	Error string
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) error {
	set, err := s.passwordSet(r.Context())
	if err != nil {
		return err
	}
	if set {
		return redirect(w, r, "/login")
	}
	return s.view.Render(w, http.StatusOK, "setup", s.page(r, "Einrichten", "", authForm{}))
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	set, err := s.passwordSet(r.Context())
	if err != nil {
		return err
	}
	if set {
		return httpError{http.StatusForbidden, "Passwort ist bereits gesetzt"}
	}
	password := r.FormValue("password")
	if len(password) < minPasswordLen {
		return s.view.Render(w, http.StatusUnprocessableEntity, "setup", s.page(r, "Einrichten", "", authForm{Error: "Mindestens 8 Zeichen"}))
	}
	if err := SetPassword(r.Context(), s.svc.Store, password); err != nil {
		return err
	}
	if err := s.startSession(w, r); err != nil {
		return err
	}
	return redirect(w, r, "/")
}

// SetPassword stores the bcrypt hash of a password.
func SetPassword(ctx context.Context, store *db.Store, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return store.Q(ctx).SetSetting(service.KeyPasswordHash, string(hash))
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) error {
	set, err := s.passwordSet(r.Context())
	if err != nil {
		return err
	}
	if !set {
		return redirect(w, r, "/setup")
	}
	if _, ok := s.session(r); ok {
		return redirect(w, r, "/")
	}
	return s.view.Render(w, http.StatusOK, "login", s.page(r, "Anmelden", "", authForm{}))
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	ip := clientIP(r)
	if s.limiter.blocked(ip) {
		w.Header().Set("Retry-After", "900")
		return s.view.Render(w, http.StatusTooManyRequests, "login", s.page(r, "Anmelden", "", authForm{Error: "Zu viele Versuche. Bitte in 15 Minuten erneut."}))
	}
	hash, err := s.svc.Store.Q(r.Context()).Setting(service.KeyPasswordHash, "")
	if err != nil {
		return err
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("password")))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrHashTooShort) {
		s.limiter.fail(ip)
		return s.view.Render(w, http.StatusUnauthorized, "login", s.page(r, "Anmelden", "", authForm{Error: "Passwort falsch"}))
	}
	if err != nil {
		return err
	}
	s.limiter.reset(ip)
	if err := s.startSession(w, r); err != nil {
		return err
	}
	return redirect(w, r, "/")
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.svc.Store.Q(r.Context()).DeleteSession(hashToken(c.Value)); err != nil {
			return err
		}
	}
	s.setCookie(w, "", -1)
	return redirect(w, r, "/login")
}

func (s *Server) passwordChange(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	hash, err := q.Setting(service.KeyPasswordHash, "")
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("current"))) != nil {
		return httpError{http.StatusForbidden, "Aktuelles Passwort falsch"}
	}
	password := r.FormValue("password")
	if len(password) < minPasswordLen {
		return badRequest("Mindestens 8 Zeichen")
	}
	if err := SetPassword(r.Context(), s.svc.Store, password); err != nil {
		return err
	}
	if err := q.DeleteAllSessions(); err != nil {
		return err
	}
	return s.startSessionAndGo(w, r)
}

func (s *Server) startSessionAndGo(w http.ResponseWriter, r *http.Request) error {
	if err := s.startSession(w, r); err != nil {
		return err
	}
	return redirect(w, r, "/settings/server")
}

// loginLimiter locks an address after too many wrong passwords.
type loginLimiter struct {
	mu    sync.Mutex
	state map[string]*loginAttempts
	now   func() time.Time
}

type loginAttempts struct {
	fails int
	until time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{state: map[string]*loginAttempts{}, now: time.Now}
}

func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.state[ip]
	if a == nil {
		return false
	}
	if l.now().After(a.until) {
		delete(l.state, ip)
		return false
	}
	return a.fails >= maxLoginFails
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.state[ip]
	if a == nil {
		a = &loginAttempts{}
		l.state[ip] = a
	}
	a.fails++
	a.until = l.now().Add(loginLockout)
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, ip)
}
