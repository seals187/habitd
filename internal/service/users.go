package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
)

// MinPasswordLen is the shortest password accepted.
const MinPasswordLen = 8

// InviteLifetime is how long an invite link stays valid.
const InviteLifetime = 7 * 24 * time.Hour

// Errors of the user administration.
var (
	ErrLastAdmin   = errors.New("last active admin")
	ErrBadUsername = errors.New("invalid username")
	ErrBadPassword = errors.New("password too short")
	ErrSetupDone   = errors.New("setup already done")
)

var usernamePattern = regexp.MustCompile(`^[\p{L}\p{N}._@-]{1,40}$`)

// HashPassword hashes a password for storage.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// RandomPassword generates a password for the command line tools.
func RandomPassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b)
}

func checkCredentials(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return ErrBadUsername
	}
	if len(password) < MinPasswordLen {
		return ErrBadPassword
	}
	return nil
}

// initUser prepares a new account: its areas and the day it started.
func (s *Service) initUser(ctx context.Context, q *db.Q, id int64) error {
	uq := q.As(id)
	if err := uq.CopyDefaultAreas(); err != nil {
		return err
	}
	if err := uq.SetSetting(KeyInstalledOn, s.Today().String()); err != nil {
		return err
	}
	return uq.SetSetting(KeyLastSettled, (s.Today() - 1).String())
}

// CreateUser adds an account with its areas and settings.
func (s *Service) CreateUser(ctx context.Context, username, password, role string) (int64, error) {
	username = strings.TrimSpace(username)
	if err := checkCredentials(username, password); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.Store.Tx(ctx, func(q *db.Q) error {
		id, err = s.addUser(ctx, q, username, hash, role)
		return err
	})
	return id, err
}

func (s *Service) addUser(ctx context.Context, q *db.Q, username, hash, role string) (int64, error) {
	id, err := q.CreateUser(model.User{Username: username, PasswordHash: hash, Role: role, CreatedAt: s.Now()})
	if err != nil {
		return 0, err
	}
	return id, s.initUser(ctx, q, id)
}

// SetupFirstAdmin creates the first account. The check runs inside the write
// transaction, so of two parallel requests only one can win. A migrated
// installation without a password claims its existing account.
func (s *Service) SetupFirstAdmin(ctx context.Context, username, password string) (int64, error) {
	username = strings.TrimSpace(username)
	if err := checkCredentials(username, password); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.Store.Tx(ctx, func(q *db.Q) error {
		n, err := q.CountUsers()
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrSetupDone
		}
		users, err := q.Users()
		if err != nil {
			return err
		}
		if len(users) > 0 {
			id = users[0].ID
			return q.ClaimUser(id, username, hash)
		}
		id, err = s.addUser(ctx, q, username, hash, model.RoleAdmin)
		return err
	})
	return id, err
}

// BootstrapOptions come from the environment.
type BootstrapOptions struct {
	AdminUser string
	Password  string
}

// Bootstrap applies the admin variables once: only while no account can sign
// in. A later start with other values changes nothing.
func (s *Service) Bootstrap(ctx context.Context, o BootstrapOptions) error {
	q := s.Store.Q(ctx)
	if name := strings.TrimSpace(o.AdminUser); name != "" {
		if err := s.nameLegacyAdmin(q, name); err != nil {
			return err
		}
	}
	if o.Password == "" {
		return nil
	}
	name := strings.TrimSpace(o.AdminUser)
	if name == "" {
		name = "admin"
	}
	_, err := s.SetupFirstAdmin(ctx, name, o.Password)
	switch {
	case errors.Is(err, ErrSetupDone):
		return nil
	case err != nil:
		return fmt.Errorf("create admin from environment: %w", err)
	}
	slog.Warn("admin account created from the environment: change its password in the app", "user", name)
	return nil
}

// nameLegacyAdmin gives a migrated account without a stored name the configured one.
func (s *Service) nameLegacyAdmin(q *db.Q, name string) error {
	flag, err := q.GlobalSetting(KeyLegacyUnnamed, "")
	if err != nil || flag == "" {
		return err
	}
	users, err := q.Users()
	if err != nil {
		return err
	}
	if len(users) > 0 {
		if err := q.RenameUser(users[0].ID, name); err != nil {
			return err
		}
	}
	return q.DeleteGlobalSetting(KeyLegacyUnnamed)
}

// wouldLoseLastAdmin reports whether taking this user out of the admins leaves none.
func wouldLoseLastAdmin(q *db.Q, id int64) (bool, error) {
	u, err := q.UserByID(id)
	if err != nil {
		return false, err
	}
	if !u.IsAdmin() || u.Disabled {
		return false, nil
	}
	n, err := q.CountActiveAdmins()
	return n <= 1, err
}

func guardLastAdmin(q *db.Q, id int64) error {
	last, err := wouldLoseLastAdmin(q, id)
	if err != nil {
		return err
	}
	if last {
		return ErrLastAdmin
	}
	return nil
}

// SetRole changes a role. The last active admin stays an admin.
func (s *Service) SetRole(ctx context.Context, id int64, role string) error {
	if role != model.RoleAdmin && role != model.RoleUser {
		return ErrNotAllowed
	}
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if role == model.RoleUser {
			if err := guardLastAdmin(q, id); err != nil {
				return err
			}
		}
		return q.SetUserRole(id, role)
	})
}

// SetDisabled blocks or unblocks a user and ends the user's sessions.
func (s *Service) SetDisabled(ctx context.Context, id int64, disabled bool) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if !disabled {
			return q.SetUserDisabled(id, false)
		}
		if err := guardLastAdmin(q, id); err != nil {
			return err
		}
		if err := q.SetUserDisabled(id, true); err != nil {
			return err
		}
		return q.DeleteUserSessions(id)
	})
}

// DeleteUser removes a user with all data.
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if err := guardLastAdmin(q, id); err != nil {
			return err
		}
		return q.DeleteUser(id)
	})
}

// ResetPassword sets a new password and ends the user's sessions.
func (s *Service) ResetPassword(ctx context.Context, id int64, password string) error {
	if len(password) < MinPasswordLen {
		return ErrBadPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if _, err := q.UserByID(id); err != nil {
			return err
		}
		if err := q.SetUserPassword(id, hash); err != nil {
			return err
		}
		return q.DeleteUserSessions(id)
	})
}

// RedeemInvite creates the account of an invite; a link works exactly once.
func (s *Service) RedeemInvite(ctx context.Context, tokenHash, username, password string) (int64, error) {
	username = strings.TrimSpace(username)
	if err := checkCredentials(username, password); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.Store.Tx(ctx, func(q *db.Q) error {
		invite, err := q.OpenInvite(tokenHash, s.Now())
		if err != nil {
			return err
		}
		if id, err = s.addUser(ctx, q, username, hash, invite.Role); err != nil {
			return err
		}
		return q.UseInvite(tokenHash, id, s.Now())
	})
	return id, err
}
