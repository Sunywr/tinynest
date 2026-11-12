// Package db owns the SQLite connection, versioned migrations and journal persistence.
// The database lives ONLY on backend host B; no browser code ever sees its path.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, no CGO required

	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var ErrMigration = errors.New("db: migration failed")

type DB struct{ *sql.DB }

// Queryer lets store functions work with either *sql.DB or *sql.Tx.
type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Open creates/opens the database with WAL, FK enforcement and a single writer.
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
	// Phase 0 already created this table; keep it as the single migration ledger.
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, description TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &DB{conn}, nil
}

// Migrate applies missing versions in order, each in its own transaction.
// Re-running is a no-op; the database is never rebuilt unconditionally.
func (d *DB) Migrate(ctx context.Context) ([]int, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var applied []int
	for _, name := range names {
		version, err := strconv.Atoi(name[:3])
		if err != nil {
			return applied, fmt.Errorf("%w: bad filename %s", ErrMigration, name)
		}
		var exists int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return applied, err
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("%w: %s: %v", ErrMigration, name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, description) VALUES(?,?)`, version, name); err != nil {
			_ = tx.Rollback()
			return applied, err
		}
		if err = tx.Commit(); err != nil {
			return applied, err
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// SeedForHousehold re-runs the idempotent category seed for a newly created household.
func SeedForHousehold(ctx context.Context, q Queryer, householdID string) error {
	body, err := migrationFS.ReadFile("migrations/007_seed.sql")
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, string(body))
	return err
}

// WithTx runs fn in one transaction; any error rolls back business rows AND journal lines together.
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

// SaveEntry persists a validated, balanced entry. Nothing is written if validation fails.
func SaveEntry(ctx context.Context, tx *sql.Tx, e *ledger.Entry, householdID, idempotencyKey, actor string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	var key any
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO journal_entries
		(id, household_id, biz_type, biz_id, occurred_on, currency, status, reversal_of, idempotency_key, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		e.ID, householdID, e.BizType, e.BizID, e.OccurredOn, e.Currency, e.Status,
		nullString(e.ReversalOf), key, actor)
	if err != nil {
		return fmt.Errorf("db: insert journal entry: %w", err)
	}
	for i, l := range e.Lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO journal_lines
			(entry_id, line_no, account_code, debit_minor, credit_minor, memo) VALUES (?,?,?,?,?,?)`,
			e.ID, i+1, l.Account, int64(l.Debit), int64(l.Credit), l.Memo); err != nil {
			return fmt.Errorf("db: insert journal line %d: %w", i+1, err)
		}
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Balance rebuilds a ledger account balance purely from journal lines.
func Balance(ctx context.Context, q Queryer, accountCode string) (money.Minor, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(debit_minor - credit_minor), 0) FROM journal_lines WHERE account_code = ?`,
		accountCode).Scan(&v)
	if err != nil {
		return 0, err
	}
	return money.Minor(v), nil
}

// Aggregate sums (debit-credit) for all account codes matching a LIKE pattern in a date range.
func Aggregate(ctx context.Context, q Queryer, pattern, from, to string) (money.Minor, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(l.debit_minor - l.credit_minor), 0)
		 FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		 WHERE l.account_code LIKE ? AND e.occurred_on >= ? AND e.occurred_on <= ?`,
		pattern, from, to).Scan(&v)
	if err != nil {
		return 0, err
	}
	return money.Minor(v), nil
}

// Cashflow sums real cash-asset movements, EXCLUDING opening balances.
// An opening balance is a one-off capital injection booked against opening equity,
// not an operating cash flow of the reporting period.
func Cashflow(ctx context.Context, q Queryer, from, to string) (money.Minor, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(l.debit_minor - l.credit_minor), 0)
		 FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		 WHERE (l.account_code LIKE '1010:%' OR l.account_code LIKE '1020:%' OR l.account_code LIKE '1030:%')
		   AND e.biz_type <> 'opening_cash'
		   AND e.occurred_on >= ? AND e.occurred_on <= ?`, from, to).Scan(&v)
	if err != nil {
		return 0, err
	}
	return money.Minor(v), nil
}

// Versions returns applied migration versions for readiness checks.
func (d *DB) Versions(ctx context.Context) ([]int, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
