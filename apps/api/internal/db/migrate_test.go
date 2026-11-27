package db

import (
	"context"
	"database/sql"
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

func TestMigrateFromZeroAppliesEveryVersion(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	expected, err := database.ExpectedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) < 2 || expected[0] != 1 || expected[1] != 2 {
		t.Fatalf("expected versions beginning [1 2] from the embedded files, got %v", expected)
	}
	applied, err := database.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(applied) != len(expected) {
		t.Fatalf("expected %d applied versions, got %v", len(expected), applied)
	}
	versions, err := database.Versions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != len(expected) {
		t.Fatalf("readiness must see every version, got %v", versions)
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

func TestStatementProvidersMigrateExistingQQMailboxAndBinding(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	files, err := database.migrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.ensureMigrationLedger(ctx); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.version > 10 {
			break
		}
		if err := database.applyMigration(ctx, file); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.recordBaseline(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO households(id,name) VALUES('existing_household','原有家庭')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO users(id,household_id,username,password_hash)
		VALUES('existing_user','existing_household','existing_user','fixture')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO credit_statement_mailboxes(household_id,user_id,email,auth_code_cipher,version,updated_at)
		VALUES('existing_household','existing_user','123@qq.com','fixture_ciphertext',3,'fixture')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO accounts(id,household_id,kind,name,ledger_code)
		VALUES('existing_card','existing_household','credit','原有卡','2010:existing_card')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO credit_statement_bindings(household_id,account_id,user_id)
		VALUES('existing_household','existing_card','existing_user')`); err != nil {
		t.Fatal(err)
	}
	applied, err := database.Migrate(ctx)
	if err != nil || len(applied) != 6 || applied[0] != 11 || applied[1] != 12 || applied[2] != 13 || applied[3] != 14 || applied[4] != 15 || applied[5] != 16 {
		t.Fatalf("apply 011 and later to existing V2: applied=%v, err=%v", applied, err)
	}
	again, err := database.Migrate(ctx)
	if err != nil || len(again) != 0 {
		t.Fatalf("repeat migration: applied=%v, err=%v", again, err)
	}
	var name, integrity string
	if err := database.QueryRow(`SELECT name FROM households WHERE id='existing_household'`).Scan(&name); err != nil || name != "原有家庭" {
		t.Fatalf("existing household = %q, err=%v", name, err)
	}
	var version int
	var cipher string
	if err := database.QueryRow(`SELECT m.version,m.auth_code_cipher FROM statement_mailbox_configs m
		JOIN statement_mailbox_card_links b ON b.household_id=m.household_id AND b.user_id=m.user_id AND b.provider=m.provider
		WHERE m.household_id='existing_household' AND m.user_id='existing_user' AND m.provider='qq' AND b.account_id='existing_card'`).
		Scan(&version, &cipher); err != nil || version != 3 || cipher != "fixture_ciphertext" {
		t.Fatalf("existing mailbox migration: version=%d err=%v", version, err)
	}
	if err := database.QueryRow(`PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("quick_check = %q, err=%v", integrity, err)
	}
	var violations int
	if err := database.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil || violations != 0 {
		t.Fatalf("foreign key violations = %d, err=%v", violations, err)
	}
}

func TestInvestmentSnapshotTablesAreUsable(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO households(id,name) VALUES('hh_inv','投资测试')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_accounts
		(id,household_id,kind,name,institution,cash_minor,dividends_minor)
		VALUES('inv_sec','hh_inv','security','证券账户','测试证券',500000,1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_holdings
		(id,household_id,account_id,symbol,name,asset_class,asset_label,quantity_micros,price_micros,
		 cost_minor,market_value_minor,unrealized_pnl_minor)
		VALUES('inv_h1','hh_inv','inv_sec','600900','长江电力','stock','股票',1000000,120000,
		 100000,120000,20000)`); err != nil {
		t.Fatal(err)
	}
	var cash, market, pnl int64
	if err := database.QueryRowContext(ctx, `
		SELECT ia.cash_minor, h.market_value_minor, h.unrealized_pnl_minor
		FROM investment_accounts ia JOIN investment_holdings h ON h.account_id=ia.id
		WHERE ia.household_id='hh_inv'`).Scan(&cash, &market, &pnl); err != nil {
		t.Fatal(err)
	}
	if cash != 500000 || market != 120000 || pnl != 20000 {
		t.Fatalf("investment snapshot values = %d/%d/%d, want 500000/120000/20000", cash, market, pnl)
	}
}

func TestCreditCycleColumnsAreNullable(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO households(id,name) VALUES('hh_cycle','测试')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO accounts(id,household_id,kind,name,ledger_code) VALUES('acc_cycle','hh_cycle','credit','测试卡','2010:acc_cycle')`); err != nil {
		t.Fatal(err)
	}
	var statement, due sql.NullInt64
	if err := database.QueryRowContext(ctx,
		`SELECT statement_day,due_day FROM accounts WHERE id='acc_cycle'`).Scan(&statement, &due); err != nil {
		t.Fatal(err)
	}
	if statement.Valid || due.Valid {
		t.Fatalf("new cycle columns must default to NULL, got %v/%v", statement, due)
	}
	if _, err := database.ExecContext(ctx,
		`UPDATE accounts SET statement_day=5,due_day=23 WHERE id='acc_cycle'`); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		`SELECT statement_day,due_day FROM accounts WHERE id='acc_cycle'`).Scan(&statement, &due); err != nil {
		t.Fatal(err)
	}
	if !statement.Valid || statement.Int64 != 5 || !due.Valid || due.Int64 != 23 {
		t.Fatalf("cycle columns not writable: %v/%v", statement, due)
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
	expected, err := database.ExpectedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(expected) {
		t.Fatalf("expected %d applied versions, got %v", len(expected), applied)
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
