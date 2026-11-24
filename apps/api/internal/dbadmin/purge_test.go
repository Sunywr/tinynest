package dbadmin

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"tinynest/api/internal/db"
)

// newTestAdmin opens a throwaway SQLite database with the V2 baseline applied.
func newTestAdmin(t *testing.T) *Admin {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "tinynest.sqlite3"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Admin{DB: database}
}

func mustExec(t *testing.T, admin *Admin, query string, args ...any) {
	t.Helper()
	if _, err := admin.DB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", firstLine(query), err)
	}
}

func firstLine(query string) string {
	if index := strings.IndexByte(query, '\n'); index >= 0 {
		return query[:index]
	}
	return query
}

// fixture is the identity of the rows seedAll inserts.
type fixture struct {
	household string
	user      string
	member    string
	account   string
	category  string
}

// seedAll populates every identity table and the V2 business tables, including a
// reversal entry that exercises the self-referencing journal_entries foreign key.
func seedAll(t *testing.T, admin *Admin) fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture{household: "hh1", user: "usr1", member: "mem1", account: "acct1"}
	mustExec(t, admin, `INSERT INTO households (id, name) VALUES (?, ?)`, f.household, "我的小家")
	mustExec(t, admin, `INSERT INTO users (id, household_id, username, password_hash, role, status) VALUES (?,?,?,?,?,?)`,
		f.user, f.household, "admin", "$argon2id$fake", "admin", "active")
	mustExec(t, admin, `INSERT INTO users (id, household_id, username, password_hash, role, status) VALUES (?,?,?,?,?,?)`,
		"usr2", f.household, "viewer", "$argon2id$fake2", "viewer", "active")
	mustExec(t, admin, `INSERT INTO sessions (id, user_id, token_hash, csrf_token, expires_at) VALUES (?,?,?,?,?)`,
		"ses1", f.user, "tokenhash", "csrf", "2099-01-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO household_members (id, household_id, name) VALUES (?,?,?)`, f.member, f.household, "我")
	mustExec(t, admin, `INSERT INTO household_user_members (user_id, member_id) VALUES (?,?)`, f.user, f.member)
	mustExec(t, admin, `INSERT INTO household_invites (token_hash, household_id, member_id, expires_at) VALUES (?,?,?,?)`,
		"invitehash", f.household, f.member, "2099-01-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO pending_registrations (username, password_hash, created_at) VALUES (?,?,?)`,
		"newbie", "$argon2id$pending", "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO site_administrators (user_id) VALUES (?)`, f.user)
	mustExec(t, admin, `INSERT INTO email_identities (email, owner, verified_at) VALUES (?,?,?)`,
		"admin@example.invalid", f.user, "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO email_challenges (id, owner, email, purpose, digest, created_at, expires_at) VALUES (?,?,?,?,?,?,?)`,
		"ch1", f.user, "admin@example.invalid", "change", "digest", "2026-11-01T00:00:00Z", "2026-11-01T00:10:00Z")
	mustExec(t, admin, `INSERT INTO mail_configuration (id, version, config_json, password_cipher, updated_at) VALUES (?,?,?,?,?)`,
		1, 1, "{}", "cipher", "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO mail_preferences (user_id, announcements) VALUES (?,?)`, f.user, 0)
	mustExec(t, admin, `INSERT INTO mail_daily_usage (day, total, announcements) VALUES (?,?,?)`, "2026-11-01", 3, 1)
	mustExec(t, admin, `INSERT INTO mail_recipient_daily_usage (day, email, total) VALUES (?,?,?)`,
		"2026-11-01", "admin@example.invalid", 2)
	mustExec(t, admin, `INSERT INTO mail_queue (id, household_id, user_id, recipient, kind, payload, status, due_at, expires_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, "mq1", f.household, f.user, "admin@example.invalid", "announcement",
		"cipher", "pending", "2026-11-01T00:00:00Z", "2026-11-02T00:00:00Z", "2026-11-01T00:00:00Z")

	if err := db.SeedForHousehold(ctx, admin.DB, f.household); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	if err := admin.DB.QueryRowContext(ctx,
		`SELECT id FROM categories WHERE household_id=? AND kind='expense' ORDER BY id LIMIT 1`, f.household).Scan(&f.category); err != nil {
		t.Fatalf("pick category: %v", err)
	}

	// Chart of accounts, two accounts and a balanced pair of entries. je2 is a
	// reversal, which exercises the self-referencing foreign key on
	// journal_entries.
	mustExec(t, admin, `INSERT INTO ledger_accounts (id, household_id, code, name, category) VALUES (?,?,?,?,?)`,
		"la1", f.household, "1010:acct1", "现金", 1)
	mustExec(t, admin, `INSERT INTO accounts (id, household_id, kind, name, ledger_code) VALUES (?,?,?,?,?)`,
		f.account, f.household, "cash", "钱包", "1010:acct1")
	mustExec(t, admin, `INSERT INTO journal_entries (id, household_id, biz_type, biz_id, occurred_on) VALUES (?,?,?,?,?)`,
		"je1", f.household, "expense", "tx1", "2026-11-01")
	mustExec(t, admin, `INSERT INTO journal_entries (id, household_id, biz_type, biz_id, occurred_on, reversal_of) VALUES (?,?,?,?,?,?)`,
		"je2", f.household, "reversal", "tx1", "2026-11-02", "je1")
	mustExec(t, admin, `INSERT INTO journal_lines (entry_id, line_no, account_code, debit_minor, credit_minor) VALUES (?,?,?,?,?)`,
		"je1", 1, "5010:"+f.category, 1000, 0)
	mustExec(t, admin, `INSERT INTO journal_lines (entry_id, line_no, account_code, debit_minor, credit_minor) VALUES (?,?,?,?,?)`,
		"je1", 2, "1010:acct1", 0, 1000)
	mustExec(t, admin, `INSERT INTO journal_lines (entry_id, line_no, account_code, debit_minor, credit_minor) VALUES (?,?,?,?,?)`,
		"je2", 1, "1010:acct1", 1000, 0)
	mustExec(t, admin, `INSERT INTO journal_lines (entry_id, line_no, account_code, debit_minor, credit_minor) VALUES (?,?,?,?,?)`,
		"je2", 2, "5010:"+f.category, 0, 1000)
	mustExec(t, admin, `INSERT INTO account_opening_balances (id, account_id, amount_minor, entry_id) VALUES (?,?,?,?)`,
		"aob1", f.account, 500, "je2")
	mustExec(t, admin, `INSERT INTO transactions (id, household_id, type, amount_minor, occurred_on, status, entry_id, account_id, category_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		"tx1", f.household, "expense", 1000, "2026-11-01", "posted", "je1", f.account, f.category)
	mustExec(t, admin, `INSERT INTO transactions (id, household_id, type, amount_minor, occurred_on, status, entry_id, account_id, category_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		"tx2", f.household, "expense", 2000, "2026-11-02", "reversed", "je2", f.account, f.category)
	mustExec(t, admin, `INSERT INTO budgets (household_id, month, amount_minor) VALUES (?,?,?)`,
		f.household, "2026-11", 5000)
	mustExec(t, admin, `INSERT INTO project_budgets(id,household_id,name,amount_minor) VALUES(?,?,?,?)`,
		"budget1", f.household, "装修", 5000)
	mustExec(t, admin, `INSERT INTO transaction_budget_links(transaction_id,budget_id) VALUES(?,?)`, "tx1", "budget1")

	// Audit trail.
	mustExec(t, admin, `INSERT INTO audit_logs (household_id, actor, action, entity, entity_id) VALUES (?,?,?,?,?)`,
		f.household, f.user, "create_transaction", "transaction", "tx1")
	return f
}

func TestPurgeTableListsAreConsistent(t *testing.T) {
	unique := map[string]bool{}
	total := 0
	for _, group := range PurgeGroups {
		if group.Name == "" || len(group.Tables) == 0 {
			t.Fatalf("group %q must have a name and at least one table", group.Name)
		}
		for _, table := range group.Tables {
			if unique[table] {
				t.Fatalf("table %q appears in more than one purge group", table)
			}
			unique[table] = true
			total++
		}
	}
	order := DeleteOrder()
	if len(order) != total {
		t.Fatalf("delete order has %d tables, groups list %d", len(order), total)
	}
	seen := map[string]bool{}
	for _, table := range order {
		if seen[table] {
			t.Fatalf("delete order repeats %q", table)
		}
		seen[table] = true
		if !unique[table] {
			t.Fatalf("delete order contains %q which is in no group", table)
		}
	}
	for _, table := range IdentityTables {
		if unique[table] {
			t.Fatalf("identity table %q is also purged", table)
		}
	}
	// Every purged table must sort after the tables it references.
	position := map[string]int{}
	for i, table := range order {
		position[table] = i
	}
	parents := map[string][]string{
		"transactions":             {"accounts", "journal_entries"},
		"journal_lines":            {"journal_entries"},
		"account_opening_balances": {"accounts", "journal_entries"},
	}
	for child, refs := range parents {
		childPos, ok := position[child]
		if !ok {
			continue
		}
		for _, parent := range refs {
			parentPos, known := position[parent]
			if !known {
				t.Fatalf("test refers to unknown parent %q", parent)
			}
			if childPos > parentPos {
				t.Fatalf("%q must be deleted before its parent %q", child, parent)
			}
		}
	}
}

func TestPurgeEmptiesBusinessDataAndKeepsIdentity(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)

	schema, err := admin.Tables(ctx)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	before, err := admin.SnapshotNow(ctx, schema)
	if err != nil {
		t.Fatalf("before snapshot: %v", err)
	}
	// Sanity: the fixture must actually populate the core business tables,
	// otherwise a passing purge proves nothing.
	for _, table := range []string{"transactions", "journal_entries", "journal_lines", "categories", "audit_logs", "household_members"} {
		if count := before.purgeCount(table); count.Rows == 0 {
			t.Fatalf("fixture did not populate %s", table)
		}
	}
	if !before.Ledger.DebitEqualsCredit {
		t.Fatal("fixture journal is not balanced")
	}

	result, err := admin.Purge(ctx, PurgeOptions{})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if !result.Committed || result.DryRun {
		t.Fatalf("expected a committed run, got %+v", result)
	}
	for _, count := range result.After.PurgeTables {
		if count.Missing {
			continue
		}
		if count.Rows != 0 {
			t.Fatalf("table %s still holds %d rows", count.Table, count.Rows)
		}
	}
	if result.After.UsersDigest != before.UsersDigest {
		t.Fatal("users table changed during purge")
	}
	if result.After.Households != 1 {
		t.Fatalf("households changed: %d", result.After.Households)
	}
	// Identity rows must be untouched, including sessions and SMTP config.
	for _, want := range before.IdentityTables {
		got := result.After.identityCount(want.Table)
		if got.Rows != want.Rows || got.Missing != want.Missing {
			t.Fatalf("identity table %s changed: before=%d after=%d", want.Table, want.Rows, got.Rows)
		}
	}
	// A fresh verify over the committed state must pass with the baseline.
	if _, err := admin.Verify(ctx, &before, false); err != nil {
		t.Fatalf("verify after purge: %v", err)
	}
}

func TestPurgeDryRunChangesNothing(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)

	result, err := admin.Purge(ctx, PurgeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if result.Committed {
		t.Fatal("dry run must not commit")
	}
	for _, count := range result.Deleted {
		if count.Missing {
			continue
		}
		if count.Rows == 0 {
			t.Fatalf("dry run reported no rows for %s; the fixture should have some", count.Table)
		}
	}
	// Nothing may have changed: a verify without a baseline still fails because
	// business rows remain.
	if _, err := admin.Verify(ctx, nil, false); err == nil {
		t.Fatal("verify must fail while business rows still exist")
	}
}

func TestPurgeRefusesMultipleHouseholds(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)
	mustExec(t, admin, `INSERT INTO households (id, name) VALUES (?, ?)`, "hh2", "另一个家")

	if _, err := admin.Purge(ctx, PurgeOptions{}); err == nil || !strings.Contains(err.Error(), "households") {
		t.Fatalf("expected a multi-household refusal, got %v", err)
	}
	result, err := admin.Purge(ctx, PurgeOptions{HouseholdID: "hh1"})
	if err != nil {
		t.Fatalf("scoped purge: %v", err)
	}
	if !result.Committed {
		t.Fatal("scoped purge did not commit")
	}
}

func TestPurgeRefusesUnbalancedJournal(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)
	mustExec(t, admin, `INSERT INTO journal_entries (id, household_id, biz_type, biz_id, occurred_on) VALUES (?,?,?,?,?)`,
		"je_bad", "hh1", "expense", "tx_bad", "2026-11-05")
	mustExec(t, admin, `INSERT INTO journal_lines (entry_id, line_no, account_code, debit_minor, credit_minor) VALUES (?,?,?,?,?)`,
		"je_bad", 1, "1010:acct1", 500, 0)

	_, err := admin.Purge(ctx, PurgeOptions{})
	if err == nil || !strings.Contains(err.Error(), "unbalanced") {
		t.Fatalf("expected an unbalanced refusal, got %v", err)
	}
	result, err := admin.Purge(ctx, PurgeOptions{AllowUnbalanced: true})
	if err != nil {
		t.Fatalf("override purge: %v", err)
	}
	if !result.Committed {
		t.Fatal("override purge did not commit")
	}
}

func TestPurgeKeepAuditPreservesHistory(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)

	result, err := admin.Purge(ctx, PurgeOptions{KeepAudit: true})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	kept := result.After.purgeCount("audit_logs")
	if kept.Rows == 0 {
		t.Fatal("keep-audit should preserve audit_logs rows")
	}
	if _, err := admin.Verify(ctx, nil, false); err == nil {
		t.Fatal("verify must report the retained audit rows")
	}
}

func TestPurgeReseedCategoriesLeavesOnlySystemDefaults(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)

	result, err := admin.Purge(ctx, PurgeOptions{ReseedCategories: true})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(result.Reseeded) != 1 || result.Reseeded[0] != "hh1" {
		t.Fatalf("unexpected reseed list %v", result.Reseeded)
	}
	if result.After.purgeCount("categories").Rows != int64(len(db.DefaultCategories)) {
		t.Fatalf("expected %d default categories, got %d",
			len(db.DefaultCategories), result.After.purgeCount("categories").Rows)
	}
	// The strict verify is rejected, the opt-in one accepts exactly the reseed.
	if _, err := admin.Verify(ctx, nil, false); err == nil {
		t.Fatal("strict verify should reject reseeded categories")
	}
	if _, err := admin.Verify(ctx, nil, true); err != nil {
		t.Fatalf("opt-in verify: %v", err)
	}
}

func TestIdentityExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := newTestAdmin(t)
	seedAll(t, source)

	var buffer bytes.Buffer
	summary, err := source.ExportIdentity(ctx, &buffer)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if summary.Rows == 0 {
		t.Fatal("export wrote no rows")
	}

	target := newTestAdmin(t)
	imported, err := target.ImportIdentity(ctx, bytes.NewReader(buffer.Bytes()), false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Rows != summary.Rows {
		t.Fatalf("row count mismatch: exported %d imported %d", summary.Rows, imported.Rows)
	}

	sourceSnap, err := source.SnapshotNow(ctx, mustSchema(t, source))
	if err != nil {
		t.Fatalf("source snapshot: %v", err)
	}
	targetSnap, err := target.SnapshotNow(ctx, mustSchema(t, target))
	if err != nil {
		t.Fatalf("target snapshot: %v", err)
	}
	if sourceSnap.UsersDigest != targetSnap.UsersDigest {
		t.Fatal("users digest differs after the round trip")
	}
	for _, want := range sourceSnap.IdentityTables {
		got := targetSnap.identityCount(want.Table)
		if want.Missing != got.Missing || want.Rows != got.Rows {
			t.Fatalf("identity table %s mismatch: source=%d target=%d", want.Table, want.Rows, got.Rows)
		}
	}

	// Importing twice must fail rather than duplicate accounts.
	if _, err := target.ImportIdentity(ctx, bytes.NewReader(buffer.Bytes()), false); err == nil {
		t.Fatal("import into a populated database must fail")
	}
	// A dry run on a populated database must fail at the same preflight.
	if _, err := target.ImportIdentity(ctx, bytes.NewReader(buffer.Bytes()), true); err == nil {
		t.Fatal("dry-run import must also refuse a populated database")
	}
}

func TestVerifyDetectsNewBusinessRow(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)
	if _, err := admin.Purge(ctx, PurgeOptions{}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	// A new category row after the purge must be caught even though the journal
	// stayed empty: the guard covers every business table, not only the ledger.
	mustExec(t, admin, `INSERT INTO categories (id, household_id, kind, name) VALUES (?,?,?,?)`,
		"cat_late", "hh1", "expense", "漏网")
	if _, err := admin.Verify(ctx, nil, false); err == nil {
		t.Fatal("verify must fail when a business row reappears")
	}
}

func mustSchema(t *testing.T, admin *Admin) SchemaInfo {
	t.Helper()
	schema, err := admin.Tables(context.Background())
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	return schema
}
