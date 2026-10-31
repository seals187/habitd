package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/seals187/habitd/migrations"
)

// legacyDB builds a single-user database as release 0.0.3 left it.
func legacyDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i, name := range []string{"001_init.sql", "002_templates.sql", "003_measures.sql"} {
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if i == 0 {
			if _, err := conn.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO settings (key, value) VALUES ('password_hash', 'old-hash'), ('admin_user', 'Julia'), ('level', '7'), ('vapid_public', 'pub')`,
		`INSERT INTO habits (id, name, kind, frequency, area_id, created_at) VALUES (5, 'Lesen', 'build', 'daily', 2, '2026-10-01T08:00:00Z')`,
		`INSERT INTO checkins (habit_id, date, created_at) VALUES (5, '2026-10-02', '2026-10-02T08:00:00Z')`,
		`INSERT INTO ledger (at, kind, amount, reason, ref) VALUES ('2026-10-02T08:00:00Z', 'xp', 10, 'checkin', 'checkin:5:2026-10-02')`,
		`INSERT INTO badges_unlocked (badge_key, unlocked_at) VALUES ('funke', '2026-10-02T08:00:00Z')`,
		`INSERT INTO sessions (token_hash, csrf, expires_at) VALUES ('tok', 'csrf', '2099-01-01T00:00:00Z')`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestMigrationAssignsLegacyData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "habitd.db")
	legacyDB(t, path)

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.Q(ctx).UserByName("julia")
	if err != nil {
		t.Fatalf("name must match in any case: %v", err)
	}
	if u.ID != 1 || !u.IsAdmin() || u.PasswordHash != "old-hash" {
		t.Fatalf("user: %+v", u)
	}

	mine := WithUser(ctx, u.ID)
	q := store.Q(mine)
	habits, err := q.Habits()
	if err != nil || len(habits) != 1 || habits[0].ID != 5 {
		t.Fatalf("habits: %v %v", habits, err)
	}
	if level, _ := q.SettingInt("level", 0); level != 7 {
		t.Fatalf("level %d", level)
	}
	badges, _ := q.UnlockedBadges()
	if len(badges) != 1 {
		t.Fatalf("badges %v", badges)
	}
	if sums, _ := q.LedgerSum("checkin:5:2026-10-02"); sums["xp"] != 10 {
		t.Fatalf("ledger %v", sums)
	}
	areas, _ := q.Areas()
	if len(areas) != 10 {
		t.Fatalf("areas %d", len(areas))
	}
	if hash, _ := q.GlobalSetting("password_hash", ""); hash != "" {
		t.Fatal("password hash stays in settings")
	}
	if pub, _ := q.GlobalSetting("vapid_public", ""); pub != "pub" {
		t.Fatal("push keys must stay global")
	}
	if id, _, err := func() (int64, string, error) {
		su, csrf, err := q.Session("tok", parseTime("2026-11-01T00:00:00Z"))
		return su.ID, csrf, err
	}(); err != nil || id != 1 {
		t.Fatalf("session: %d %v", id, err)
	}

	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %v %v", backups, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// A second start finds nothing to do: no new backup, no changed data.
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if again, _ := filepath.Glob(path + ".bak-*"); len(again) != 1 {
		t.Fatalf("backups: %v", again)
	}
	if users, _ := store.Q(ctx).Users(); len(users) != 1 {
		t.Fatalf("users: %v", users)
	}
}

func TestFreshDatabaseHasNoUser(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "habitd.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if users, _ := store.Q(ctx).Users(); len(users) != 0 {
		t.Fatalf("users: %v", users)
	}
	if backups, _ := filepath.Glob(path + ".bak-*"); len(backups) != 0 {
		t.Fatalf("no backup for a new database: %v", backups)
	}
	// Queries without a user see nothing, not everything.
	if areas, _ := store.Q(ctx).Areas(); len(areas) != 0 {
		t.Fatalf("areas: %v", areas)
	}
}
