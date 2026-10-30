// Package db wraps SQLite: connection, migrations and queries.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"slices"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // database driver

	"github.com/seals187/habitd/migrations"
)

// Store is the open database.
type Store struct {
	sql *sql.DB
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
	s := &Store{sql: conn}
	if err := s.migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.sql.Close() }

// Ping checks the connection.
func (s *Store) Ping(ctx context.Context) error { return s.sql.PingContext(ctx) }

// Q returns a query runner outside of any transaction.
func (s *Store) Q(ctx context.Context) *Q { return &Q{r: s.sql, ctx: ctx} }

// Tx runs fn in a write transaction.
func (s *Store) Tx(ctx context.Context, fn func(q *Q) error) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&Q{r: tx, ctx: ctx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
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
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad number: %w", name, err)
		}
		var done int
		if err := s.sql.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?`, version).Scan(&done); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if done > 0 {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		err = s.Tx(ctx, func(q *Q) error {
			if err := q.exec(string(body)); err != nil {
				return err
			}
			return q.exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version)
		})
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func (q *Q) exec(query string, args ...any) error {
	if _, err := q.r.ExecContext(q.ctx, query, args...); err != nil {
		return fmt.Errorf("exec %q: %w", firstLine(query), err)
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
