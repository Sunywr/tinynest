package dbadmin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/platform/config"
)

// Admin performs destructive and read-only operator operations against one
// already-open database.
type Admin struct {
	DB *db.DB
}

// Open connects with exactly the settings the API server uses.
func Open(cfg config.Database) (*Admin, error) {
	switch cfg.Driver {
	case config.DriverMySQL:
		handle, err := db.OpenMySQL(cfg.MySQLSocket, cfg.MySQLDatabase, cfg.MySQLUser, cfg.MySQLPassword)
		if err != nil {
			return nil, err
		}
		return &Admin{DB: handle}, nil
	case config.DriverSQLite:
		handle, err := db.Open(cfg.SQLitePath)
		if err != nil {
			return nil, err
		}
		return &Admin{DB: handle}, nil
	default:
		return nil, errors.New("dbadmin: unsupported database driver")
	}
}

// Close releases the connection.
func (a *Admin) Close() error { return a.DB.Close() }

// SchemaInfo is the set of tables the connected database actually contains.
type SchemaInfo struct {
	tables map[string]bool
	all    []string
}

// Has reports whether the table exists.
func (s SchemaInfo) Has(table string) bool { return s.tables[table] }

// All returns every table name in sorted order.
func (s SchemaInfo) All() []string {
	out := make([]string, len(s.all))
	copy(out, s.all)
	return out
}

// MissingPurgeTables lists purge targets absent from this schema.
func (s SchemaInfo) MissingPurgeTables() []string {
	var missing []string
	for _, table := range PurgeTableSet() {
		if !s.tables[table] {
			missing = append(missing, table)
		}
	}
	return missing
}

// TableCount records one table's row count. Missing is set when the current
// schema does not contain the table at all (for example a table introduced by a
// later migration than the deployed binary).
type TableCount struct {
	Table   string `json:"table"`
	Group   string `json:"group,omitempty"`
	Rows    int64  `json:"rows"`
	Missing bool   `json:"missing,omitempty"`
}

// Ledger is the balance invariant captured before and after a purge.
type Ledger struct {
	Entries           int64 `json:"entries"`
	Lines             int64 `json:"lines"`
	DebitMinor        int64 `json:"debit_minor"`
	CreditMinor       int64 `json:"credit_minor"`
	UnbalancedEntries int64 `json:"unbalanced_entries"`
	OrphanLines       int64 `json:"orphan_lines"`
	JournalIsEmpty    bool  `json:"journal_is_empty"`
	DebitEqualsCredit bool  `json:"debit_equals_credit"`
}

// Snapshot is a full, comparable picture of the data a purge must leave intact
// or empty.
type Snapshot struct {
	CapturedAt     string       `json:"captured_at"`
	Driver         string       `json:"driver"`
	Database       string       `json:"database,omitempty"`
	Migrations     []int        `json:"migrations"`
	Households     int64        `json:"households"`
	UsersDigest    string       `json:"users_digest"`
	PurgeTables    []TableCount `json:"purge_tables"`
	IdentityTables []TableCount `json:"identity_tables"`
	Ledger         Ledger       `json:"ledger"`
}

// PurgeOptions controls a purge run.
type PurgeOptions struct {
	// HouseholdID scopes the run when more than one household exists. The
	// production invariant is exactly one household; a multi-household database
	// must name the one it means.
	HouseholdID string
	// DryRun reports what would be deleted without changing anything.
	DryRun bool
	// ReseedCategories recreates the default category set after the purge so the
	// deployed UI stays able to record a transaction.
	ReseedCategories bool
	// KeepAudit preserves audit_logs and schedule_runs (still clears the rest).
	KeepAudit bool
	// AllowUnbalanced proceeds even when the journal is already unbalanced. The
	// default refuses, so a purge can never hide a pre-existing ledger defect.
	AllowUnbalanced bool
}

// PurgeResult is what the operator records as evidence.
type PurgeResult struct {
	DryRun        bool         `json:"dry_run"`
	Before        Snapshot     `json:"before"`
	After         Snapshot     `json:"after"`
	Deleted       []TableCount `json:"deleted"`
	MissingTables []string     `json:"missing_tables,omitempty"`
	Warnings      []string     `json:"warnings,omitempty"`
	Reseeded      []string     `json:"reseeded_households,omitempty"`
	Committed     bool         `json:"committed"`
}

// Purge deletes every business row while leaving identity and permission rows
// untouched. All deletions happen in one transaction; any failed assertion rolls
// the whole run back.
func (a *Admin) Purge(ctx context.Context, opts PurgeOptions) (PurgeResult, error) {
	schema, err := a.Tables(ctx)
	if err != nil {
		return PurgeResult{}, err
	}

	before, err := a.snapshot(ctx, a.DB, schema)
	if err != nil {
		return PurgeResult{}, err
	}

	result := PurgeResult{DryRun: opts.DryRun, Before: before, MissingTables: schema.MissingPurgeTables()}
	for _, table := range PurgeTableSet() {
		count := before.purgeCount(table)
		result.Deleted = append(result.Deleted, TableCount{Table: table, Group: GroupOf(table), Rows: count.Rows, Missing: count.Missing})
	}

	if households := before.Households; households != 1 && opts.HouseholdID == "" {
		return result, fmt.Errorf("dbadmin: refusing to purge %d households; the production invariant is 1. Pass --household=<id> to select one", households)
	}
	if opts.HouseholdID != "" {
		exists, err := a.householdExists(ctx, opts.HouseholdID)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("dbadmin: household %q does not exist", opts.HouseholdID)
		}
	}
	if !opts.AllowUnbalanced {
		switch {
		case before.Ledger.OrphanLines != 0:
			return result, fmt.Errorf("dbadmin: %d journal lines have no entry; refusing to purge over an inconsistent journal (pass -allow-unbalanced to override)", before.Ledger.OrphanLines)
		case !before.Ledger.DebitEqualsCredit:
			return result, fmt.Errorf("dbadmin: journal is already unbalanced (debit %d, credit %d); refusing to purge (pass -allow-unbalanced to override)", before.Ledger.DebitMinor, before.Ledger.CreditMinor)
		case before.Ledger.UnbalancedEntries != 0:
			return result, fmt.Errorf("dbadmin: %d journal entries are unbalanced; refusing to purge (pass -allow-unbalanced to override)", before.Ledger.UnbalancedEntries)
		}
	}

	if opts.DryRun {
		result.After = before
		return result, nil
	}

	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		for _, statement := range DetachSelfReferences() {
			fields := strings.Fields(statement)
			if len(fields) < 4 || !schema.Has(fields[1]) {
				continue
			}
			// A table can survive across schema generations with a different
			// column set (e.g. V2 categories no longer has parent_id). Only run
			// the detach statement when the column actually exists.
			column := strings.SplitN(fields[3], "=", 2)[0]
			if !a.hasColumn(ctx, tx, fields[1], column) {
				continue
			}
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("dbadmin: detach self reference: %w", err)
			}
		}
		for _, table := range DeleteOrder() {
			if !schema.Has(table) {
				continue
			}
			if opts.KeepAudit && (table == "audit_logs" || table == "schedule_runs") {
				continue
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil { // #nosec G201 -- table names come from a fixed allow-list
				return fmt.Errorf("dbadmin: delete from %s: %w", table, err)
			}
		}
		// Prove emptiness on the exact data the transaction will commit. The
		// assertion runs before any category reseed so a strict "all business
		// tables are zero" invariant is what gets verified.
		emptied, err := a.snapshot(ctx, tx, schema)
		if err != nil {
			return err
		}
		allow := map[string]bool{}
		if opts.KeepAudit {
			allow["audit_logs"] = true
			allow["schedule_runs"] = true
		}
		if err := assertPurgedExcept(emptied, allow); err != nil {
			return err
		}
		if err := assertIdentityKept(before, emptied); err != nil {
			return err
		}
		if opts.ReseedCategories {
			households, err := survivingHouseholds(ctx, tx, opts.HouseholdID)
			if err != nil {
				return err
			}
			for _, household := range households {
				if err := db.SeedForHousehold(ctx, tx, household); err != nil {
					return fmt.Errorf("dbadmin: reseed categories for %s: %w", household, err)
				}
			}
			result.Reseeded = households
		}
		return nil
	})
	if err != nil {
		result.After = result.Before
		return result, err
	}
	// The final snapshot is taken after the transaction so it reflects exactly
	// what was committed (including an optional category reseed).
	after, err := a.snapshot(ctx, a.DB, schema)
	if err != nil {
		result.After = result.Before
		return result, err
	}

	result.After = after
	result.Committed = true
	if len(result.MissingTables) > 0 {
		result.Warnings = append(result.Warnings,
			"以下表在当前 schema 中不存在，已跳过（若部署版本较新，请确认是否需要纳入清理）："+strings.Join(result.MissingTables, ", "))
	}
	return result, nil
}

// Verify re-reads the database and compares it with a recorded snapshot.
// allowSeededCategories permits the default category rows that an optional
// purge reseed writes; every other business table must still be empty.
func (a *Admin) Verify(ctx context.Context, baseline *Snapshot, allowSeededCategories bool) (Snapshot, error) {
	schema, err := a.Tables(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	current, err := a.snapshot(ctx, a.DB, schema)
	if err != nil {
		return Snapshot{}, err
	}
	allow := map[string]bool{}
	if allowSeededCategories {
		allow["categories"] = true
	}
	if err := assertPurgedExcept(current, allow); err != nil {
		return current, err
	}
	if baseline != nil {
		if err := assertIdentityKept(*baseline, current); err != nil {
			return current, err
		}
		if baseline.UsersDigest != current.UsersDigest {
			return current, errors.New("dbadmin: verification failed: the users table changed since the baseline capture")
		}
	}
	return current, nil
}

// SnapshotNow captures the current state without changing anything.
func (a *Admin) SnapshotNow(ctx context.Context, schema SchemaInfo) (Snapshot, error) {
	return a.snapshot(ctx, a.DB, schema)
}

// --- schema discovery -------------------------------------------------------

// Tables lists the tables that exist in the connected database.
func (a *Admin) Tables(ctx context.Context) (SchemaInfo, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table'"
	if a.DB.IsMySQL() {
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE'"
	}
	rows, err := a.DB.QueryContext(ctx, query)
	if err != nil {
		return SchemaInfo{}, fmt.Errorf("dbadmin: list tables: %w", err)
	}
	defer rows.Close()
	info := SchemaInfo{tables: map[string]bool{}}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return SchemaInfo{}, err
		}
		info.tables[name] = true
		info.all = append(info.all, name)
	}
	if err := rows.Err(); err != nil {
		return SchemaInfo{}, err
	}
	sort.Strings(info.all)
	return info, nil
}

// --- snapshots --------------------------------------------------------------

func (a *Admin) snapshot(ctx context.Context, q db.Queryer, schema SchemaInfo) (Snapshot, error) {
	snap := Snapshot{CapturedAt: time.Now().UTC().Format(time.RFC3339)}
	if a.DB.IsMySQL() {
		snap.Driver = config.DriverMySQL
	} else {
		snap.Driver = config.DriverSQLite
	}

	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM households").Scan(&snap.Households); err != nil {
		return snap, fmt.Errorf("dbadmin: count households: %w", err)
	}
	digest, err := a.usersDigest(ctx, q)
	if err != nil {
		return snap, err
	}
	snap.UsersDigest = digest

	versions, err := appliedVersions(ctx, q)
	if err != nil {
		return snap, err
	}
	snap.Migrations = versions

	for _, table := range PurgeTableSet() {
		count := TableCount{Table: table, Group: GroupOf(table)}
		if schema.Has(table) {
			if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count.Rows); err != nil { // #nosec G201 -- allow-listed table
				return snap, fmt.Errorf("dbadmin: count %s: %w", table, err)
			}
		} else {
			count.Missing = true
		}
		snap.PurgeTables = append(snap.PurgeTables, count)
	}
	for _, table := range IdentityTables {
		count := TableCount{Table: table, Group: "身份与权限"}
		if schema.Has(table) {
			if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count.Rows); err != nil { // #nosec G201 -- allow-listed table
				return snap, fmt.Errorf("dbadmin: count %s: %w", table, err)
			}
		} else {
			count.Missing = true
		}
		snap.IdentityTables = append(snap.IdentityTables, count)
	}
	ledger, err := a.ledger(ctx, q)
	if err != nil {
		return snap, err
	}
	snap.Ledger = ledger
	return snap, nil
}

func (snap Snapshot) purgeCount(table string) TableCount {
	for _, c := range snap.PurgeTables {
		if c.Table == table {
			return c
		}
	}
	return TableCount{Table: table, Group: GroupOf(table), Missing: true}
}

func (snap Snapshot) identityCount(table string) TableCount {
	for _, c := range snap.IdentityTables {
		if c.Table == table {
			return c
		}
	}
	return TableCount{Table: table, Missing: true}
}

func (a *Admin) ledger(ctx context.Context, q db.Queryer) (Ledger, error) {
	var out Ledger
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM journal_entries").Scan(&out.Entries); err != nil {
		return out, fmt.Errorf("dbadmin: count journal entries: %w", err)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM journal_lines").Scan(&out.Lines); err != nil {
		return out, fmt.Errorf("dbadmin: count journal lines: %w", err)
	}
	if err := q.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(debit_minor),0), COALESCE(SUM(credit_minor),0) FROM journal_lines").
		Scan(&out.DebitMinor, &out.CreditMinor); err != nil {
		return out, fmt.Errorf("dbadmin: sum journal lines: %w", err)
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
		SELECT entry_id FROM journal_lines GROUP BY entry_id
		HAVING SUM(debit_minor) <> SUM(credit_minor)
	) unbalanced`).Scan(&out.UnbalancedEntries); err != nil {
		return out, fmt.Errorf("dbadmin: check unbalanced entries: %w", err)
	}
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM journal_lines l LEFT JOIN journal_entries e ON e.id=l.entry_id WHERE e.id IS NULL").
		Scan(&out.OrphanLines); err != nil {
		return out, fmt.Errorf("dbadmin: check orphan lines: %w", err)
	}
	out.DebitEqualsCredit = out.DebitMinor == out.CreditMinor
	out.JournalIsEmpty = out.Entries == 0 && out.Lines == 0
	return out, nil
}

func (a *Admin) usersDigest(ctx context.Context, q db.Queryer) (string, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, household_id, username, password_hash, role, status FROM users ORDER BY id")
	if err != nil {
		return "", fmt.Errorf("dbadmin: read users: %w", err)
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var id, household, username, passwordHash, role, status string
		if err := rows.Scan(&id, &household, &username, &passwordHash, &role, &status); err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1e", id, household, username, passwordHash, role, status)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *Admin) householdExists(ctx context.Context, id string) (bool, error) {
	var count int64
	if err := a.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM households WHERE id=?", id).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

// survivingHouseholds returns the households to reseed. Seeding is idempotent in
// both dialects, so returning every household is safe.
func survivingHouseholds(ctx context.Context, tx *sql.Tx, only string) ([]string, error) {
	query, args := "SELECT id FROM households ORDER BY id", []any{}
	if only != "" {
		query, args = "SELECT id FROM households WHERE id=? ORDER BY id", []any{only}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func appliedVersions(ctx context.Context, q db.Queryer) ([]int, error) {
	rows, err := q.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("dbadmin: read schema_migrations: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, rows.Err()
}

// hasColumn reports whether a table in the current schema has the given column.
// Older and newer schema generations share table names with different columns,
// so detach statements must be verified instead of assumed.
func (a *Admin) hasColumn(ctx context.Context, q db.Queryer, table, column string) bool {
	var query string
	var args []any
	if a.DB.IsMySQL() {
		query = `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`
		args = []any{table, column}
	} else {
		query = `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`
		args = []any{table, column}
	}
	var count int
	return q.QueryRowContext(ctx, query, args...).Scan(&count) == nil && count > 0
}

// --- assertions -------------------------------------------------------------

func assertPurged(snap Snapshot) error {
	return assertPurgedExcept(snap, nil)
}

func assertPurgedExcept(snap Snapshot, allow map[string]bool) error {
	var remaining []string
	for _, count := range snap.PurgeTables {
		if count.Missing || allow[count.Table] {
			continue
		}
		if count.Rows != 0 {
			remaining = append(remaining, fmt.Sprintf("%s=%d", count.Table, count.Rows))
		}
	}
	if len(remaining) > 0 {
		return fmt.Errorf("dbadmin: purge left business rows: %s", strings.Join(remaining, " "))
	}
	if !snap.Ledger.JournalIsEmpty {
		return fmt.Errorf("dbadmin: journal is not empty after purge (entries=%d lines=%d)", snap.Ledger.Entries, snap.Ledger.Lines)
	}
	return nil
}

func assertIdentityKept(before, after Snapshot) error {
	var broken []string
	for _, want := range before.IdentityTables {
		got := after.identityCount(want.Table)
		if got.Missing != want.Missing || got.Rows != want.Rows {
			broken = append(broken, fmt.Sprintf("%s: before=%d after=%d", want.Table, want.Rows, got.Rows))
		}
	}
	if len(broken) > 0 {
		return fmt.Errorf("dbadmin: identity rows changed: %s", strings.Join(broken, " "))
	}
	if before.UsersDigest != "" && after.UsersDigest != before.UsersDigest {
		return errors.New("dbadmin: the users table changed; the purge must not touch accounts, roles or password hashes")
	}
	return nil
}

// Unclassified reports tables that are neither purged nor protected, so the
// operator notices schema drift instead of silently keeping unknown data.
func (snap Snapshot) Unclassified(all []string) []string {
	known := map[string]bool{"schema_migrations": true}
	for _, table := range PurgeTableSet() {
		known[table] = true
	}
	for _, table := range IdentityTables {
		known[table] = true
	}
	var out []string
	for _, table := range all {
		if !known[table] {
			out = append(out, table)
		}
	}
	sort.Strings(out)
	return out
}
