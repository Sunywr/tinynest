package dbadmin

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"tinynest/api/internal/db"
)

// openMySQLTestAdmin connects to a disposable MySQL database named by envVar.
// The CI drill script provisions one fresh database per test; the test is
// skipped when no socket is configured so the normal `go test ./...` run on a
// workstation without MySQL stays green.
func openMySQLTestAdmin(t *testing.T, databaseEnv string) *Admin {
	t.Helper()
	socket := os.Getenv("TINYNEST_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("TINYNEST_TEST_MYSQL_SOCKET is not set: MySQL drill skipped")
	}
	name := os.Getenv(databaseEnv)
	if name == "" {
		t.Skip(databaseEnv + " is not set: MySQL drill skipped")
	}
	passwordPath := os.Getenv("TINYNEST_TEST_MYSQL_PASSWORD_FILE")
	raw, err := os.ReadFile(passwordPath)
	if err != nil {
		t.Fatalf("read MySQL password file: %v", err)
	}
	handle, err := db.OpenMySQL(socket, name, os.Getenv("TINYNEST_TEST_MYSQL_USER"), strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("open MySQL %s: %v", name, err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	if _, err := handle.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate MySQL %s: %v", name, err)
	}
	return &Admin{DB: handle}
}

// TestMySQLPurgeDrill is the production rehearsal: it exercises the exact
// destructive path against the production dialect, including the FK-safe delete
// order, the in-transaction emptiness assertion and the identity comparison.
func TestMySQLPurgeDrill(t *testing.T) {
	admin := openMySQLTestAdmin(t, "TINYNEST_TEST_MYSQL_DATABASE")
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
	if before.purgeCount("transactions").Rows == 0 || before.purgeCount("budgets").Rows == 0 {
		t.Fatal("fixture did not populate the MySQL bookkeeping tables")
	}
	// The bank-reported statements and the card links reference accounts, so they
	// are what proves the delete order holds on the production dialect.
	for _, table := range []string{"card_statements", "card_statement_sources", "statement_mailbox_card_links", "credit_statement_bindings", "investment_accounts"} {
		if before.purgeCount(table).Rows == 0 {
			t.Fatalf("fixture did not populate %s", table)
		}
	}
	for _, table := range PreservedConfigTables {
		if count := before.preservedCount(table); count.Rows != 1 {
			t.Fatalf("%s before purge = %d, want 1", table, count.Rows)
		}
	}

	result, err := admin.Purge(ctx, PurgeOptions{})
	if err != nil {
		t.Fatalf("purge over MySQL: %v", err)
	}
	if !result.Committed {
		t.Fatal("purge did not commit")
	}
	for _, count := range result.After.PurgeTables {
		if count.Missing {
			continue
		}
		if count.Rows != 0 {
			t.Fatalf("table %s still holds %d rows after the MySQL purge", count.Table, count.Rows)
		}
	}
	if result.After.UsersDigest != before.UsersDigest {
		t.Fatal("users table changed during the MySQL purge")
	}
	for _, table := range PreservedConfigTables {
		if count := result.After.preservedCount(table); count.Rows != 1 {
			t.Fatalf("%s survived the MySQL purge with %d rows, want 1", table, count.Rows)
		}
	}
	if _, err := admin.Verify(ctx, &before, false); err != nil {
		t.Fatalf("verify after MySQL purge: %v", err)
	}
}

// TestMySQLIdentityCutoverDrill rehearses the V2 cutover on the production
// dialect: export identity rows from the populated database and load them into a
// second, freshly created V2 schema.
func TestMySQLIdentityCutoverDrill(t *testing.T) {
	source := openMySQLTestAdmin(t, "TINYNEST_TEST_MYSQL_DATABASE")
	ctx := context.Background()
	seedAll(t, source)

	var buffer bytes.Buffer
	summary, err := source.ExportIdentity(ctx, &buffer)
	if err != nil {
		t.Fatalf("export identity: %v", err)
	}
	if summary.Rows == 0 {
		t.Fatal("export wrote no identity rows")
	}

	target := openMySQLTestAdmin(t, "TINYNEST_TEST_MYSQL_DATABASE_ALT")
	imported, err := target.ImportIdentity(ctx, bytes.NewReader(buffer.Bytes()), false)
	if err != nil {
		t.Fatalf("import identity: %v", err)
	}
	if imported.Rows != summary.Rows {
		t.Fatalf("row count mismatch: exported %d imported %d", summary.Rows, imported.Rows)
	}

	sourceSnap, err := source.SnapshotNow(ctx, mustSchemaInfo(t, source))
	if err != nil {
		t.Fatalf("source snapshot: %v", err)
	}
	targetSnap, err := target.SnapshotNow(ctx, mustSchemaInfo(t, target))
	if err != nil {
		t.Fatalf("target snapshot: %v", err)
	}
	if sourceSnap.UsersDigest != targetSnap.UsersDigest {
		t.Fatal("users digest differs after the MySQL cutover rehearsal")
	}
	for _, want := range sourceSnap.IdentityTables {
		got := targetSnap.identityCount(want.Table)
		if want.Missing != got.Missing || want.Rows != got.Rows {
			t.Fatalf("identity table %s mismatch after import: source=%d target=%d", want.Table, want.Rows, got.Rows)
		}
	}
	// The target must still be a valid V2 baseline and refuse unknown versions.
	if versions, err := appliedVersions(ctx, target.DB); err != nil || len(versions) == 0 {
		t.Fatalf("target migration ledger unreadable: %v (%v)", err, versions)
	}
}

func mustSchemaInfo(t *testing.T, admin *Admin) SchemaInfo {
	t.Helper()
	schema, err := admin.Tables(context.Background())
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	return schema
}
