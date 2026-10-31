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
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

const (
	sessionCookie   = "habitd_session"
	sessionLifetime = 30 * 24 * time.Hour
	defaultUser     = "admin"
	maxLoginFails   = 5
	loginLockout    = 15 * time.Minute
)

// dummyHash keeps a sign-in with an unknown name as slow as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("habitd"), bcrypt.DefaultCost)

type ctxKey int

const (
	csrfKey ctxKey = 1
	userKey ctxKey = 2
)

func csrfFrom(r *http.Request) string {
	v, _ := r.Context().Value(csrfKey).(string)
	return v
}

// currentUser returns the signed-in user; the zero value on public pages.
func currentUser(r *http.Request) model.User {
	u, _ := r.Context().Value(userKey).(model.User)
	return u
}

func withUser(r *http.Request, u model.User, csrf string) *http.Request {
	ctx := db.WithUser(r.Context(), u.ID)
	ctx = context.WithValue(ctx, userKey, u)
	return r.WithContext(context.WithValue(ctx, csrfKey, csrf))
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
	case "/healthz", "/login", "/setup", "/signup", "/sw.js", "/manifest.webmanifest", "/offline":
		return true
	}
	return strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/invite/")
}

func (s *Server) hasUsers(ctx context.Context) (bool, error) {
	n, err := s.svc.Store.Q(ctx).CountUsers()
	return n > 0, err
}

// authenticate enforces login and CSRF tokens.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.bearerUser(r); ok {
			next.ServeHTTP(w, withUser(r, u, ""))
			return
		}
		u, csrf, ok := s.session(r)
		if publicPath(r.URL.Path) {
			if ok {
				r = withUser(r, u, csrf)
			}
			next.ServeHTTP(w, r)
			return
		}
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
		next.ServeHTTP(w, withUser(r, u, csrf))
	})
}

// bearerUser maps the API token to the first active admin: the JSON API stems
// from the single-user time and has one token per instance.
func (s *Server) bearerUser(r *http.Request) (model.User, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || s.cfg.APIToken == "" || !strings.HasPrefix(r.URL.Path, "/api/") ||
		subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.APIToken)) != 1 {
		return model.User{}, false
	}
	users, err := s.svc.Store.Q(r.Context()).Users()
	if err != nil {
		return model.User{}, false
	}
	for _, u := range users {
		if u.IsAdmin() && !u.Disabled {
			return u, true
		}
	}
	return model.User{}, false
}

func (s *Server) session(r *http.Request) (model.User, string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return model.User{}, "", false
	}
	u, csrf, err := s.svc.Store.Q(r.Context()).Session(hashToken(c.Value), time.Now())
	return u, csrf, err == nil
}

func (s *Server) rejectAnonymous(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	target := "/login"
	if exists, err := s.hasUsers(r.Context()); err == nil && !exists {
		target = "/setup"
	}
	_ = redirect(w, r, target)
}

func (s *Server) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token := randomToken()
	expires := time.Now().Add(sessionLifetime)
	q := s.svc.Store.Q(r.Context())
	if err := q.CreateSession(userID, hashToken(token), randomToken(), expires); err != nil {
		return err
	}
	if err := q.TouchLogin(userID, time.Now()); err != nil {
		return err
	}
	s.setCookie(w, token, int(sessionLifetime.Seconds()))
	return nil
}

type authForm struct {
	Error  string
	User   string
	Intro  string
	Signup bool
}

// credentialError turns a validation error into text for the form.
func credentialError(err error) (string, bool) {
	switch {
	case errors.Is(err, service.ErrBadUsername):
		return "Der Benutzername darf nur Buchstaben, Zahlen und . _ @ - enthalten", true
	case errors.Is(err, service.ErrBadPassword):
		return "Mindestens 8 Zeichen", true
	case errors.Is(err, db.ErrUserExists):
		return "Dieser Benutzername ist vergeben", true
	}
	return "", false
}

func notFound() error { return httpError{http.StatusNotFound, "Nicht gefunden"} }

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) error {
	done, err := s.hasUsers(r.Context())
	if err != nil {
		return err
	}
	if done {
		return notFound()
	}
	return s.view.Render(w, http.StatusOK, "setup", s.page(r, "Einrichten", "", authForm{User: defaultUser}))
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	ip := clientIP(r)
	if s.limiter.blocked(ip) {
		return httpError{http.StatusTooManyRequests, "Zu viele Versuche. Bitte in 15 Minuten erneut."}
	}
	user := strings.TrimSpace(r.FormValue("username"))
	if user == "" {
		user = defaultUser
	}
	id, err := s.svc.SetupFirstAdmin(r.Context(), user, r.FormValue("password"))
	if errors.Is(err, service.ErrSetupDone) {
		s.limiter.fail(ip)
		return notFound()
	}
	if message, ok := credentialError(err); ok {
		s.limiter.fail(ip)
		return s.view.Render(w, http.StatusUnprocessableEntity, "setup", s.page(r, "Einrichten", "", authForm{Error: message, User: user}))
	}
	if err != nil {
		return err
	}
	if err := s.startSession(w, r, id); err != nil {
		return err
	}
	return redirect(w, r, "/")
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) error {
	exists, err := s.hasUsers(r.Context())
	if err != nil {
		return err
	}
	if !exists {
		return redirect(w, r, "/setup")
	}
	if currentUser(r).ID != 0 {
		return redirect(w, r, "/")
	}
	return s.view.Render(w, http.StatusOK, "login", s.page(r, "Anmelden", "", authForm{Signup: s.cfg.AllowSignup}))
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
	name := strings.TrimSpace(r.FormValue("username"))
	if s.limiter.blocked(ip) {
		w.Header().Set("Retry-After", "900")
		return s.view.Render(w, http.StatusTooManyRequests, "login", s.page(r, "Anmelden", "", authForm{Error: "Zu viele Versuche. Bitte in 15 Minuten erneut.", User: name}))
	}
	u, err := s.svc.Store.Q(r.Context()).UserByName(name)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	// The password is always checked, so a wrong name takes just as long.
	known := err == nil
	hash := dummyHash
	if known {
		hash = []byte(u.PasswordHash)
	}
	err = bcrypt.CompareHashAndPassword(hash, []byte(r.FormValue("password")))
	if !known || u.Disabled || errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrHashTooShort) {
		s.limiter.fail(ip)
		return s.view.Render(w, http.StatusUnauthorized, "login", s.page(r, "Anmelden", "", authForm{Error: "Benutzername oder Passwort falsch", User: name, Signup: s.cfg.AllowSignup}))
	}
	if err != nil {
		return err
	}
	s.limiter.reset(ip)
	if err := s.startSession(w, r, u.ID); err != nil {
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
	u := currentUser(r)
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(r.FormValue("current"))) != nil {
		return httpError{http.StatusForbidden, "Aktuelles Passwort falsch"}
	}
	if err := s.svc.ResetPassword(r.Context(), u.ID, r.FormValue("password")); err != nil {
		if errors.Is(err, service.ErrBadPassword) {
			return badRequest("Mindestens 8 Zeichen")
		}
		return err
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		return err
	}
	return redirect(w, r, "/settings/server")
}

func (s *Server) signupForm(w http.ResponseWriter, r *http.Request) error {
	if !s.cfg.AllowSignup {
		return notFound()
	}
	return s.view.Render(w, http.StatusOK, "signup", s.page(r, "Registrieren", "", authForm{Intro: "Leg dein Konto an."}))
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) error {
	if !s.cfg.AllowSignup {
		return notFound()
	}
	return s.register(w, r, "Leg dein Konto an.", func(user, password string) (int64, error) {
		return s.svc.CreateUser(r.Context(), user, password, model.RoleUser)
	})
}

func (s *Server) inviteForm(w http.ResponseWriter, r *http.Request) error {
	if s.limiter.blocked(clientIP(r)) {
		return httpError{http.StatusTooManyRequests, "Zu viele Versuche. Bitte in 15 Minuten erneut."}
	}
	if _, err := s.svc.Store.Q(r.Context()).OpenInvite(hashToken(r.PathValue("token")), s.svc.Now()); err != nil {
		s.limiter.fail(clientIP(r))
		return err
	}
	return s.view.Render(w, http.StatusOK, "signup", s.page(r, "Einladung", "", authForm{Intro: "Du wurdest eingeladen. Leg dein Konto an."}))
}

func (s *Server) inviteAccept(w http.ResponseWriter, r *http.Request) error {
	return s.register(w, r, "Du wurdest eingeladen. Leg dein Konto an.", func(user, password string) (int64, error) {
		return s.svc.RedeemInvite(r.Context(), hashToken(r.PathValue("token")), user, password)
	})
}

// register runs the shared part of sign-up and invite.
func (s *Server) register(w http.ResponseWriter, r *http.Request, intro string, create func(user, password string) (int64, error)) error {
	ip := clientIP(r)
	if s.limiter.blocked(ip) {
		return httpError{http.StatusTooManyRequests, "Zu viele Versuche. Bitte in 15 Minuten erneut."}
	}
	user := strings.TrimSpace(r.FormValue("username"))
	id, err := create(user, r.FormValue("password"))
	if message, ok := credentialError(err); ok {
		s.limiter.fail(ip)
		return s.view.Render(w, http.StatusUnprocessableEntity, "signup", s.page(r, "Registrieren", "", authForm{Intro: intro, Error: message, User: user}))
	}
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.limiter.fail(ip)
		}
		return err
	}
	if err := s.startSession(w, r, id); err != nil {
		return err
	}
	return redirect(w, r, "/")
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
