package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

type userRow struct {
	model.User
	Level    int
	Habits   int
	XP       int
	LastSeen string
	Self     bool
}

type inviteRow struct {
	Token string
	Role  string
	Until string
}

// oneTime shows a secret that cannot be read again later.
type oneTime struct {
	Label string
	Value string
}

type usersData struct {
	Rows    []userRow
	Invites []inviteRow
	Error   string
	Secret  *oneTime
}

func (s *Server) usersPage(w http.ResponseWriter, r *http.Request, status int, message string, secret *oneTime) error {
	q := s.svc.Store.Q(r.Context())
	users, err := q.Users()
	if err != nil {
		return err
	}
	stats, err := q.UserStats()
	if err != nil {
		return err
	}
	invites, err := q.Invites()
	if err != nil {
		return err
	}
	d := usersData{Error: message, Secret: secret}
	rules := s.svc.Rules()
	for _, u := range users {
		stored, err := q.As(u.ID).SettingInt(service.KeyLevel, 1)
		if err != nil {
			return err
		}
		st := stats[u.ID]
		row := userRow{User: u, Habits: st.Habits, XP: st.XP, Level: rules.ProgressFor(st.XP, stored).Level, LastSeen: "—", Self: u.ID == currentUser(r).ID}
		if !u.LastLogin.IsZero() {
			row.LastSeen = view.ShortDate(u.LastLogin)
		}
		d.Rows = append(d.Rows, row)
	}
	for _, i := range invites {
		if i.UsedAt.IsZero() && i.ExpiresAt.After(s.svc.Now()) {
			d.Invites = append(d.Invites, inviteRow{Token: i.TokenHash, Role: i.Role, Until: view.ShortDate(i.ExpiresAt)})
		}
	}
	// A page shown after a post must not repeat the post on reload.
	w.Header().Set("HX-Replace-Url", "/admin/users")
	return s.view.Render(w, status, "admin_users", s.adminPage(r, "Benutzer", "users", d))
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) error {
	return s.usersPage(w, r, http.StatusOK, "", nil)
}

func formRole(r *http.Request) string {
	if r.FormValue("role") == model.RoleAdmin {
		return model.RoleAdmin
	}
	return model.RoleUser
}

func (s *Server) adminUserCreate(w http.ResponseWriter, r *http.Request) error {
	name := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	var secret *oneTime
	if password == "" {
		password = service.RandomPassword()
		secret = &oneTime{Label: "Passwort für " + name, Value: password}
	}
	_, err := s.svc.CreateUser(r.Context(), name, password, formRole(r))
	if message, ok := credentialError(err); ok {
		return s.usersPage(w, r, http.StatusUnprocessableEntity, message, nil)
	}
	if err != nil {
		return err
	}
	return s.usersPage(w, r, http.StatusOK, "", secret)
}

func (s *Server) adminUserRole(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.SetRole(r.Context(), id, formRole(r)); err != nil {
		return err
	}
	return redirect(w, r, "/admin/users")
}

func (s *Server) adminUserDisabled(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.SetDisabled(r.Context(), id, r.FormValue("disabled") == "on"); err != nil {
		return err
	}
	return redirect(w, r, "/admin/users")
}

func (s *Server) adminUserPassword(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	u, err := s.svc.Store.Q(r.Context()).UserByID(id)
	if err != nil {
		return err
	}
	password := service.RandomPassword()
	if err := s.svc.ResetPassword(r.Context(), id, password); err != nil {
		return err
	}
	if u.ID == currentUser(r).ID {
		if err := s.startSession(w, r, u.ID); err != nil {
			return err
		}
	}
	return s.usersPage(w, r, http.StatusOK, "", &oneTime{Label: "Neues Passwort für " + u.Username, Value: password})
}

func (s *Server) adminUserDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	u, err := s.svc.Store.Q(r.Context()).UserByID(id)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(r.FormValue("confirm")), u.Username) {
		return s.usersPage(w, r, http.StatusUnprocessableEntity, "Bitte gib zur Bestätigung den Benutzernamen ein", nil)
	}
	if err := s.svc.DeleteUser(r.Context(), id); err != nil {
		return err
	}
	return redirect(w, r, "/admin/users")
}

func inviteURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/invite/" + token
}

func (s *Server) adminInviteCreate(w http.ResponseWriter, r *http.Request) error {
	token := randomToken()
	invite := db.Invite{TokenHash: hashToken(token), CreatedBy: currentUser(r).ID, Role: formRole(r), ExpiresAt: s.svc.Now().Add(service.InviteLifetime)}
	if err := s.svc.Store.Q(r.Context()).CreateInvite(invite); err != nil {
		return err
	}
	return s.usersPage(w, r, http.StatusOK, "", &oneTime{Label: "Einladungslink, " + strconv.Itoa(int(service.InviteLifetime.Hours()/24)) + " Tage gültig", Value: inviteURL(r, token)})
}

func (s *Server) adminInviteDelete(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.Store.Q(r.Context()).DeleteInvite(r.PathValue("token")); err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	return redirect(w, r, "/admin/users")
}
