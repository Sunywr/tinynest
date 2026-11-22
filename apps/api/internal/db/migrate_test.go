package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestMigrateFromZeroAppliesBothVersions(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	expected, err := database.ExpectedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != 2 || expected[0] != 1 || expected[1] != 2 {
		t.Fatalf("expected versions [1 2] from the embedded files, got %v", expected)
	}
	applied, err := database.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("expected 2 applied versions, got %v", applied)
	}
	versions, err := database.Versions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("readiness must see both versions, got %v", versions)
	}
	var baseline string
	if err := database.QueryRow(`SELECT value FROM schema_meta WHERE meta_key='baseline'`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != "v2" {
		t.Fatalf("schema_meta.baseline = %q, want v2", baseline)
	}
	// Re-running is a no-op.
	again, err := database.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second migrate applied %v, want nothing", again)
	}
}

func TestSeedForHouseholdIsIdempotent(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO households(id,name) VALUES('hh_seed','测试')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := SeedForHousehold(ctx, database, "hh_seed"); err != nil {
			t.Fatalf("seed run %d: %v", i+1, err)
		}
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE household_id='hh_seed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(DefaultCategories) {
		t.Fatalf("seeded %d categories, want %d", count, len(DefaultCategories))
	}
	// Stable ids: `cat_<household>_<code>`.
	var id string
	if err := database.QueryRowContext(ctx, `SELECT id FROM categories WHERE household_id='hh_seed' AND name='工资'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != "cat_hh_seed_salary" {
		t.Fatalf("stable category id = %q", id)
	}
}

// A V1 database (schema_migrations rows but no V2 schema_meta baseline) must be
// refused instead of silently altered.
func TestBaselineGuardRejectsV1Database(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if err := database.ensureMigrationLedger(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, description, checksum) VALUES(1,'001_base.sql','old')`); err != nil {
		t.Fatal(err)
	}
	_, err := database.Migrate(ctx)
	if !errors.Is(err, ErrBaseline) {
		t.Fatalf("expected ErrBaseline, got %v", err)
	}
}

// A real V1 ledger has no checksum column. It must produce the explicit
// baseline refusal (not an SQL error) and stay untouched.
func TestV1ShapedLedgerIsRefusedUntouched(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, description TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, description) VALUES(1,'001_base.sql')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); !errors.Is(err, ErrBaseline) {
		t.Fatalf("expected ErrBaseline for a V1-shaped ledger, got %v", err)
	}
	if database.ledgerHasChecksumColumn(ctx) {
		t.Fatal("Migrate must not alter the V1 ledger")
	}
	var rows int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("V1 ledger rows changed: %d", rows)
	}
}

// An EMPTY legacy ledger holds no history, so it is safe to rebuild before the
// first V2 migration row is written.
func TestEmptyLegacyLedgerIsRebuilt(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, description TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	applied, err := database.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate over an empty legacy ledger: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("expected both versions applied, got %v", applied)
	}
	if !database.ledgerHasChecksumColumn(ctx) {
		t.Fatal("ledger was not rebuilt with the checksum column")
	}
}

// A migrated database whose baseline marker was lost must also be refused.
func TestBaselineGuardRequiresV2Marker(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE schema_meta SET value='v1' WHERE meta_key='baseline'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); !errors.Is(err, ErrBaseline) {
		t.Fatalf("expected ErrBaseline after tampering with the baseline marker, got %v", err)
	}
}

// Modifying an already-applied migration file is detected by the checksum.
func TestChecksumTamperIsDetected(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE schema_migrations SET checksum='deadbeef' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); !errors.Is(err, ErrMigration) {
		t.Fatalf("expected ErrMigration for a checksum mismatch, got %v", err)
	}
}

// A version recorded by a different binary must abort startup, not be skipped.
func TestUnknownAppliedVersionIsRefused(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, description, checksum) VALUES(99,'999_other.sql','x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); !errors.Is(err, ErrMigration) {
		t.Fatalf("expected ErrMigration for an unknown applied version, got %v", err)
	}
}

func TestSplitSQLStatements(t *testing.T) {
	script := `
		CREATE TABLE IF NOT EXISTS a (id INT); -- trailing comment
		CREATE TABLE b (note VARCHAR(20) DEFAULT 'semi;colon');
		INSERT INTO b(note) VALUES ("double;quoted");
	`
	statements := splitSQLStatements(script)
	if len(statements) != 3 {
		t.Fatalf("expected 3 statements, got %d: %v", len(statements), statements)
	}
	if statements[1] != `CREATE TABLE b (note VARCHAR(20) DEFAULT 'semi;colon')` {
		t.Fatalf("single-quoted semicolon split the statement: %q", statements[1])
	}
	if statements[2] != `INSERT INTO b(note) VALUES ("double;quoted")` {
		t.Fatalf("double-quoted semicolon split the statement: %q", statements[2])
	}
}
