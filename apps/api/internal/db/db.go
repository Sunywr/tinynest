// Package db owns the database connections, the single V2 migration path and
// the journal persistence helpers.
//
// V2 is a BASELINE RESET: there are exactly two embedded migration files per
// dialect (001_core.sql and 002_bookkeeping.sql). A database created by the V1
// binary is never upgraded in place; the migrator refuses to touch it (see the
// baseline guard in migrate.go) and the operator follows
// docs/plans/v2-cutover-runbook.md instead.
//
// The database lives ONLY on backend host B; no browser code ever sees its path.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver, no CGO required
)

var ErrMigration = errors.New("db: migration failed")

// DB wraps one connection pool. SQLite and MySQL share the same helpers: every
// caller branches on IsMySQL() only where the SQL dialects genuinely differ.
type DB struct {
	*sql.DB
	mysql bool
}

// Queryer lets store functions work with either *sql.DB or *sql.Tx.
type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Open creates/opens the SQLite database with WAL, FK enforcement and a single
// writer. Migrations are NOT applied here; the composition root calls Migrate.
func Open(path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("db: empty database path")
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("db: refusing a symlinked database file")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; one connection also guarantees PRAGMAs apply everywhere.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("db: %s: %w", pragma, err)
		}
	}
	return &DB{DB: conn}, nil
}

// WithTx runs fn in one transaction; any error rolls back business rows AND
// journal lines together. No caller may claim success without this commit.
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
