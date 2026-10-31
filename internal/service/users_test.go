package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
)

func bare(t *testing.T) *env {
	t.Helper()
	e := setup(t)
	if err := e.s.DeleteUserForTest(e.ctx); err != nil {
		t.Fatal(err)
	}
	e.ctx = context.Background()
	return e
}

// DeleteUserForTest empties the users table of the test setup.
func (s *Service) DeleteUserForTest(ctx context.Context) error {
	return s.Store.Q(ctx).DeleteUser(db.UserID(ctx))
}

func TestParallelSetupCreatesOneAdmin(t *testing.T) {
	e := bare(t)
	var wg sync.WaitGroup
	wins := make(chan int64, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "admin" + string(rune('a'+i))
			if id, err := e.s.SetupFirstAdmin(e.ctx, name, "first-password"); err == nil {
				wins <- id
			} else if !errors.Is(err, ErrSetupDone) {
				t.Errorf("setup: %v", err)
			}
		}()
	}
	wg.Wait()
	close(wins)
	if len(wins) != 1 {
		t.Fatalf("%d winners", len(wins))
	}
	users, _ := e.s.Store.Q(e.ctx).Users()
	if len(users) != 1 || !users[0].IsAdmin() {
		t.Fatalf("users: %+v", users)
	}
}

func TestBootstrapOnlyOnFirstStart(t *testing.T) {
	e := bare(t)
	if err := e.s.Bootstrap(e.ctx, BootstrapOptions{AdminUser: "anna", Password: "first-password"}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Bootstrap(e.ctx, BootstrapOptions{AdminUser: "other", Password: "second-password"}); err != nil {
		t.Fatal(err)
	}
	users, _ := e.s.Store.Q(e.ctx).Users()
	if len(users) != 1 || users[0].Username != "anna" || !users[0].IsAdmin() {
		t.Fatalf("users: %+v", users)
	}
	hash := users[0].PasswordHash
	if _, err := e.s.SetupFirstAdmin(e.ctx, "third", "third-password"); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("setup after bootstrap: %v", err)
	}
	if u, _ := e.s.Store.Q(e.ctx).UserByID(users[0].ID); u.PasswordHash != hash {
		t.Fatal("password was overwritten")
	}
}

func TestLastAdminIsProtected(t *testing.T) {
	e := setup(t)
	admin := db.UserID(e.ctx)
	ctx := context.Background()
	if err := e.s.SetRole(ctx, admin, model.RoleUser); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote: %v", err)
	}
	if err := e.s.SetDisabled(ctx, admin, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable: %v", err)
	}
	if err := e.s.DeleteUser(ctx, admin); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete: %v", err)
	}
	second, err := e.s.CreateUser(ctx, "zweite", "second-password", model.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.SetDisabled(ctx, second, true); err != nil {
		t.Fatal(err)
	}
	// A blocked admin does not count as the remaining one.
	if err := e.s.SetRole(ctx, admin, model.RoleUser); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote with a blocked second admin: %v", err)
	}
	if err := e.s.SetDisabled(ctx, second, false); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SetRole(ctx, admin, model.RoleUser); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUserRemovesAllData(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 3)
	if err := e.s.Check(e.ctx, id, e.s.Today()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := e.s.CreateUser(ctx, "zweite", "second-password", model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := e.s.DeleteUser(ctx, db.UserID(e.ctx)); err != nil {
		t.Fatal(err)
	}
	if h, _ := e.q().Habits(); len(h) != 0 {
		t.Fatalf("habits left: %v", h)
	}
	if b := e.bal(); b.XP != 0 {
		t.Fatalf("ledger left: %+v", b)
	}
	if badges, _ := e.q().UnlockedBadges(); len(badges) != 0 {
		t.Fatalf("badges left: %v", badges)
	}
}

func TestInviteWorksOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q := e.s.Store.Q(ctx)
	if err := q.CreateInvite(db.Invite{TokenHash: "h", Role: model.RoleAdmin, ExpiresAt: e.now.Add(InviteLifetime)}); err != nil {
		t.Fatal(err)
	}
	id, err := e.s.RedeemInvite(ctx, "h", "neu", "invite-password")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := q.UserByID(id)
	if !u.IsAdmin() {
		t.Fatalf("role of the invite: %+v", u)
	}
	if _, err := e.s.RedeemInvite(ctx, "h", "noch-einer", "invite-password"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("second use: %v", err)
	}
	if err := q.CreateInvite(db.Invite{TokenHash: "alt", Role: model.RoleUser, ExpiresAt: e.now.Add(-1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.RedeemInvite(ctx, "alt", "zu-spaet", "invite-password"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expired: %v", err)
	}
}

func TestSettleIsPerUser(t *testing.T) {
	e := setup(t)
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	other, err := e.s.CreateUser(context.Background(), "zweite", "second-password", model.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.Store.Q(db.WithUser(context.Background(), other)).SetSetting(KeyLastSettled, (e.s.Today() - 5).String()); err != nil {
		t.Fatal(err)
	}
	ctx := db.WithUser(context.Background(), other)
	if err := e.s.Settle(ctx); err != nil {
		t.Fatal(err)
	}
	if last, _ := e.s.Store.Q(ctx).Setting(KeyLastSettled, ""); last != (e.s.Today() - 1).String() {
		t.Fatalf("second user was skipped: %s", last)
	}
}

func TestExportMovesBetweenUsers(t *testing.T) {
	e := setup(t)
	if err := e.s.Seed(e.ctx); err != nil {
		t.Fatal(err)
	}
	before := e.bal()
	dump, err := e.q().DumpAll()
	if err != nil {
		t.Fatal(err)
	}
	other, err := e.s.CreateUser(context.Background(), "zweite", "second-password", model.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	ctx := db.WithUser(context.Background(), other)
	if err := e.s.Import(ctx, dump); err != nil {
		t.Fatal(err)
	}
	habits, _ := e.s.Store.Q(ctx).Habits()
	if len(habits) != 5 {
		t.Fatalf("copied habits: %d", len(habits))
	}
	mine, _ := e.q().Habits()
	if len(mine) != 5 || mine[0].ID == habits[0].ID {
		t.Fatalf("source must stay untouched: %v", mine)
	}
	bal, _ := e.s.Store.Q(ctx).Balances(e.s.Today())
	if bal != before {
		t.Fatalf("before %+v after %+v", before, bal)
	}
	xp, _, err := e.s.Store.Q(ctx).HabitEarnings(habits[0].ID)
	if err != nil || xp == 0 {
		t.Fatalf("ledger refs must follow the new ids: %d %v", xp, err)
	}
}
