// Package store is lazyleet's local persistence layer: a single SQLite database
// (pure-Go driver, no CGO) holding the problem cache, problem detail cache,
// study plans, submission history, and workspace state.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies any
// pending migrations, and returns a ready Store. Pass ":memory:" for tests.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(ON)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc's driver is safe for concurrent use, but WAL + a single writer is
	// simplest to reason about.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DB exposes the underlying handle for packages that run their own queries.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SchemaVersion returns the highest applied migration index (0 if none).
func (s *Store) SchemaVersion() (int, error) {
	return s.schemaVersion(context.Background())
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER NOT NULL,
        applied_at INTEGER NOT NULL DEFAULT (unixepoch())
    )`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	current, err := s.schemaVersion(ctx)
	if err != nil {
		return err
	}

	for i := current; i < len(migrations); i++ {
		version := i + 1
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}
