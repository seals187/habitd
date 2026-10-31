package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/model"
)

// ErrUserExists is returned for a username that is already taken.
var ErrUserExists = errors.New("user exists")

const userColumns = `id, username, display_name, password_hash, role, created_at, last_login_at, disabled`

func scanUser(s scanner) (model.User, error) {
	var u model.User
	var created string
	var login sql.NullString
	err := s.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Role, &created, &login, &u.Disabled)
	u.CreatedAt = parseTime(created)
	if login.Valid {
		u.LastLogin = parseTime(login.String)
	}
	return u, err
}

func (q *Q) user(where string, arg any) (model.User, error) {
	u, err := scanUser(q.r.QueryRowContext(q.ctx, `SELECT `+userColumns+` FROM users WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// UserByID returns a user.
func (q *Q) UserByID(id int64) (model.User, error) { return q.user(`id = ?`, id) }

// UserByName returns a user, ignoring case.
func (q *Q) UserByName(name string) (model.User, error) { return q.user(`username = ?`, name) }

// Users lists all users.
func (q *Q) Users() ([]model.User, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers counts the users that can sign in.
func (q *Q) CountUsers() (int, error) {
	var n int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM users WHERE password_hash <> ''`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// CountActiveAdmins counts the admins that are not disabled.
func (q *Q) CountActiveAdmins() (int, error) {
	var n int
	err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND disabled = 0`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

// CreateUser stores a new user and returns its id.
func (q *Q) CreateUser(u model.User) (int64, error) {
	res, err := q.r.ExecContext(q.ctx, `INSERT INTO users (username, display_name, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.Username, u.DisplayName, u.PasswordHash, u.Role, formatTime(u.CreatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrUserExists
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return res.LastInsertId()
}

// ClaimUser gives the account of a migrated installation its first login.
func (q *Q) ClaimUser(id int64, username, hash string) error {
	return q.exec(`UPDATE users SET username = ?, password_hash = ? WHERE id = ?`, username, hash, id)
}

// RenameUser changes a username.
func (q *Q) RenameUser(id int64, username string) error {
	return q.exec(`UPDATE users SET username = ? WHERE id = ?`, username, id)
}

// SetUserRole changes a role.
func (q *Q) SetUserRole(id int64, role string) error {
	return q.exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
}

// SetUserDisabled blocks or unblocks a user.
func (q *Q) SetUserDisabled(id int64, disabled bool) error {
	return q.exec(`UPDATE users SET disabled = ? WHERE id = ?`, b2i(disabled), id)
}

// SetUserPassword stores a new password hash.
func (q *Q) SetUserPassword(id int64, hash string) error {
	return q.exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
}

// TouchLogin records a sign-in.
func (q *Q) TouchLogin(id int64, at time.Time) error {
	return q.exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, formatTime(at), id)
}

// DeleteUser removes a user; foreign keys take all of the user's data along.
func (q *Q) DeleteUser(id int64) error {
	// Redemptions hang on rewards and the user alike, the order keeps both happy.
	return q.exec(`DELETE FROM users WHERE id = ?`, id)
}

// UserStat holds the figures an admin may see: no habit contents.
type UserStat struct {
	Habits int
	XP     int
}

// UserStats returns the figures of all users.
func (q *Q) UserStats() (map[int64]UserStat, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT u.id,
		(SELECT count(*) FROM habits h WHERE h.user_id = u.id AND h.archived = 0),
		(SELECT coalesce(sum(l.amount), 0) FROM ledger l WHERE l.user_id = u.id AND l.kind = 'xp')
		FROM users u`)
	if err != nil {
		return nil, fmt.Errorf("user stats: %w", err)
	}
	defer rows.Close()
	out := map[int64]UserStat{}
	for rows.Next() {
		var id int64
		var st UserStat
		if err := rows.Scan(&id, &st.Habits, &st.XP); err != nil {
			return nil, fmt.Errorf("scan user stat: %w", err)
		}
		out[id] = st
	}
	return out, rows.Err()
}

// DefaultAreas lists the global start values of the areas.
func (q *Q) DefaultAreas() ([]model.Area, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT id, key, name, color, icon, sort FROM areas WHERE user_id IS NULL ORDER BY sort`)
	if err != nil {
		return nil, fmt.Errorf("list default areas: %w", err)
	}
	defer rows.Close()
	var out []model.Area
	for rows.Next() {
		var a model.Area
		if err := rows.Scan(&a.ID, &a.Key, &a.Name, &a.Color, &a.Icon, &a.Sort); err != nil {
			return nil, fmt.Errorf("scan area: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CopyDefaultAreas gives the current user a copy of the global areas.
func (q *Q) CopyDefaultAreas() error {
	return q.exec(`INSERT INTO areas (user_id, key, name, color, icon, sort)
		SELECT ?, key, name, color, icon, sort FROM areas WHERE user_id IS NULL ORDER BY sort`, q.uid)
}

// CreateSession stores a session of a user.
func (q *Q) CreateSession(userID int64, tokenHash, csrf string, expires time.Time) error {
	return q.exec(`INSERT INTO sessions (user_id, token_hash, csrf, expires_at) VALUES (?, ?, ?, ?)`,
		userID, tokenHash, csrf, expires.UTC().Format(timeLayout))
}

// Session returns the user and CSRF token of a valid session. The user is read
// on every request, so role changes and blocks apply at once.
func (q *Q) Session(tokenHash string, now time.Time) (model.User, string, error) {
	var csrf string
	var id int64
	err := q.r.QueryRowContext(q.ctx, `SELECT csrf, user_id FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now.UTC().Format(timeLayout)).Scan(&csrf, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, "", ErrNotFound
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("get session: %w", err)
	}
	u, err := q.UserByID(id)
	if err != nil {
		return u, "", err
	}
	if u.Disabled {
		return u, "", ErrNotFound
	}
	return u, csrf, nil
}

// DeleteSession removes a session.
func (q *Q) DeleteSession(tokenHash string) error {
	return q.exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
}

// DeleteUserSessions signs a user out everywhere.
func (q *Q) DeleteUserSessions(userID int64) error {
	return q.exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
}

// Invite is a one-time sign-up link.
type Invite struct {
	TokenHash string
	CreatedBy int64
	Role      string
	ExpiresAt time.Time
	UsedBy    int64
	UsedAt    time.Time
}

// CreateInvite stores an invite.
func (q *Q) CreateInvite(i Invite) error {
	return q.exec(`INSERT INTO invites (token, created_by, role, expires_at) VALUES (?, ?, ?, ?)`,
		i.TokenHash, nullable(i.CreatedBy), i.Role, formatTime(i.ExpiresAt))
}

// OpenInvite returns an unused invite that has not expired.
func (q *Q) OpenInvite(tokenHash string, now time.Time) (Invite, error) {
	i := Invite{TokenHash: tokenHash}
	var expires string
	err := q.r.QueryRowContext(q.ctx, `SELECT role, expires_at FROM invites WHERE token = ? AND used_at IS NULL`, tokenHash).Scan(&i.Role, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, fmt.Errorf("get invite: %w", err)
	}
	i.ExpiresAt = parseTime(expires)
	if !i.ExpiresAt.After(now) {
		return i, ErrNotFound
	}
	return i, nil
}

// UseInvite marks an invite as used.
func (q *Q) UseInvite(tokenHash string, userID int64, at time.Time) error {
	return q.exec(`UPDATE invites SET used_by = ?, used_at = ? WHERE token = ? AND used_at IS NULL`, userID, formatTime(at), tokenHash)
}

// Invites lists invites, newest first.
func (q *Q) Invites() ([]Invite, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT token, role, expires_at, used_at FROM invites ORDER BY expires_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var i Invite
		var expires string
		var used sql.NullString
		if err := rows.Scan(&i.TokenHash, &i.Role, &expires, &used); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		i.ExpiresAt = parseTime(expires)
		if used.Valid {
			i.UsedAt = parseTime(used.String)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DeleteInvite withdraws an invite.
func (q *Q) DeleteInvite(tokenHash string) error {
	return q.exec(`DELETE FROM invites WHERE token = ?`, tokenHash)
}
