package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/service"
)

func adminEnv(t *testing.T) (*service.Service, func(...string) (string, error)) {
	t.Helper()
	ctx := context.Background()
	store, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := service.New(store, time.UTC)
	return svc, func(args ...string) (string, error) {
		var out bytes.Buffer
		err := adminCommand(ctx, svc, args, &out)
		return out.String(), err
	}
}

func TestAdminCommands(t *testing.T) {
	svc, run := adminEnv(t)
	ctx := context.Background()
	if _, err := run("create", "julia", "--admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("create", "bert"); err != nil {
		t.Fatal(err)
	}
	out, err := run("list")
	if err != nil || !strings.Contains(out, "julia") || !strings.Contains(out, "bert") {
		t.Fatalf("list: %s %v", out, err)
	}
	q := svc.Store.Q(ctx)
	if u, _ := q.UserByName("bert"); u.IsAdmin() {
		t.Fatal("bert must start as a user")
	}

	if _, err := run("demote", "julia"); err == nil || !strings.Contains(err.Error(), "last active admin") {
		t.Fatalf("demote the last admin: %v", err)
	}
	if u, _ := q.UserByName("julia"); !u.IsAdmin() {
		t.Fatal("julia lost the role")
	}
	if _, err := run("promote", "bert"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("demote", "JULIA"); err != nil {
		t.Fatal(err)
	}
	if u, _ := q.UserByName("julia"); u.IsAdmin() {
		t.Fatal("demote had no effect")
	}

	out, err = run("reset-password", "bert")
	if err != nil {
		t.Fatal(err)
	}
	password := out[strings.LastIndex(out, " ")+1:]
	u, _ := q.UserByName("bert")
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(strings.TrimSpace(password))); err != nil {
		t.Fatalf("printed password does not work: %v", err)
	}

	if _, err := run("reset-password", "nobody"); err == nil {
		t.Fatal("unknown user")
	}
	if _, err := run("bogus", "bert"); err == nil {
		t.Fatal("unknown command")
	}
	if _, err := run(); err == nil {
		t.Fatal("no command")
	}
}
