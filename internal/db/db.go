// Package db wraps SQLite: connection, migrations and queries.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // database driver

	"github.com/seals187/habitd/migrations"
)

// Store is the open database.
type Store struct {
	sql  *sql.DB
	path string
}

// runner is the part of database/sql shared by *sql.DB and *sql.Tx.
type runner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Q runs queries, either directly or inside a transaction.
type Q struct {
	r   runner
	ctx context.Context
	uid int64 // the user all user data queries are limited to, 0 matches nothing
}

type userKey struct{}

// WithUser limits every query that runs under ctx to the data of one user.
func WithUser(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

// UserID returns the user set with WithUser, 0 if there is none.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey{}).(int64)
	return id
}

// Open opens the database at path, enables WAL and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	params := url.Values{}
	params.Add("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "synchronous(NORMAL)")
	params.Add("_txlock", "immediate")
	conn, err := sql.Open("sqlite", "file:"+path+"?"+params.Encode())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(1) // keeps idle memory low
	s := &Store{sql: conn, path: path}
	if err := s.migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

// As returns the same runner limited to another user.
func (q *Q) As(id int64) *Q { return &Q{r: q.r, ctx: q.ctx, uid: id} }

// Close closes the database.
func (s *Store) Close() error { return s.sql.Close() }

// Ping checks the connection.
func (s *Store) Ping(ctx context.Context) error { return s.sql.PingContext(ctx) }

// Q returns a query runner outside of any transaction.
func (s *Store) Q(ctx context.Context) *Q { return &Q{r: s.sql, ctx: ctx, uid: UserID(ctx)} }

// Tx runs fn in a write transaction.
func (s *Store) Tx(ctx context.Context, fn func(q *Q) error) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&Q{r: tx, ctx: ctx, uid: UserID(ctx)}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// migrate applies new migrations. Foreign keys stay off meanwhile, because
// rebuilding a table would otherwise cascade into its child tables. The pragma
// is per connection and has no effect inside a transaction.
func (s *Store) migrate(ctx context.Context) error {
	conn, err := s.sql.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`) }()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	var applied int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return fmt.Errorf("count migrations: %w", err)
	}
	backedUp := false
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad number: %w", name, err)
		}
		var done int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?`, version).Scan(&done); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if done > 0 {
			continue
		}
		if applied > 0 && !backedUp {
			if err := s.backup(ctx, conn); err != nil {
				return err
			}
			backedUp = true
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, conn, string(body), version); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

// backup copies an existing database before migrations change it.
func (s *Store) backup(ctx context.Context, conn *sql.Conn) error {
	target := s.path + ".bak-" + time.Now().UTC().Format("20060102-150405")
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return fmt.Errorf("back up database: %w", err)
	}
	slog.Info("database backed up before migration", "file", target)
	return nil
}

// BackupTo writes a consistent copy of the database to path.
func (s *Store) BackupTo(ctx context.Context, path string) error {
	if _, err := s.sql.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("back up database: %w", err)
	}
	return nil
}

// Size returns the size of the database file in bytes.
func (s *Store) Size() (int64, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, body string, version int) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key violation")
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Q) exec(query string, args ...any) error {
	if _, err := q.r.ExecContext(q.ctx, query, args...); err != nil {
		return fmt.Errorf("exec %q: %w", firstLine(query), err)
	}
	return nil
}

// execOne runs a write that must hit a row of the current user: a foreign id
// ends as ErrNotFound, not as a silent success.
func (q *Q) execOne(query string, args ...any) error {
	res, err := q.r.ExecContext(q.ctx, query, args...)
	if err != nil {
		return fmt.Errorf("exec %q: %w", firstLine(query), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
