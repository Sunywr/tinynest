package dbadmin

import (
	"context"
	"testing"
)

// seedStatementRows populates the tables added by migrations 005, 009-012: the
// read-only investment snapshot, the personal statement mailboxes and the
// bank-reported card statements. They all hang off households, users or accounts,
// so they decide whether a purge can delete accounts at all.
func seedStatementRows(t *testing.T, admin *Admin, f fixture) {
	t.Helper()
	mustExec(t, admin, `INSERT INTO accounts (id, household_id, kind, name, ledger_code) VALUES (?,?,?,?,?)`,
		"acctcc", f.household, "credit", "浦发信用卡(4321)", "2202:acctcc")
	mustExec(t, admin, `INSERT INTO statement_mailbox_configs
		(household_id, user_id, provider, email, auth_code_cipher, version, updated_at) VALUES (?,?,?,?,?,?,?)`,
		f.household, f.user, "qq", "admin@example.invalid", "cipher", 1, "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO statement_mailbox_card_links
		(household_id, account_id, user_id, provider) VALUES (?,?,?,?)`,
		f.household, "acctcc", f.user, "qq")
	// The pre-011 mailbox pair stays in the schema after 011 copies it, so a purge
	// has to cope with its rows too: the binding references accounts, the mailbox
	// holds the credential and is preserved.
	mustExec(t, admin, `INSERT INTO credit_statement_mailboxes
		(household_id, user_id, email, auth_code_cipher, version, updated_at) VALUES (?,?,?,?,?,?)`,
		f.household, f.user, "admin@example.invalid", "cipher", 1, "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO credit_statement_bindings (household_id, account_id, user_id) VALUES (?,?,?)`,
		f.household, "acctcc", f.user)
	mustExec(t, admin, `INSERT INTO card_statements
		(id, household_id, account_id, period_start_on, period_end_on, due_on, currency, amount_due_minor, status, review_reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"cs1", f.household, "acctcc", "2026-10-02", "2026-11-01", "2026-11-25", "CNY", 120000,
		"pending_review", "", "2026-11-01T00:00:00Z", "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO card_statement_sources
		(id, household_id, statement_id, kind, provider, user_id, uid, received_on, digest, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"css1", f.household, "cs1", "email", "qq", f.user, 42, "2026-11-01", "digest1", "2026-11-01T00:00:00Z")
	mustExec(t, admin, `INSERT INTO investment_accounts (id, household_id, kind, name, cash_minor) VALUES (?,?,?,?,?)`,
		"inv1", f.household, "security", "证券账户", 500000)
	mustExec(t, admin, `INSERT INTO investment_holdings (id, household_id, account_id, symbol, name, market_value_minor)
		VALUES (?,?,?,?,?,?)`, "invh1", f.household, "inv1", "600900", "长江电力", 120000)
}

// A purge must be able to empty the credit-card statement chain. card_statements// and statement_mailbox_card_links both reference accounts, so if they are in
// neither the purge list nor the identity whitelist, DELETE FROM accounts fails
// on the foreign key and the whole runbook aborts.
func TestPurgeClearsCardStatementTables(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	seedAll(t, admin)

	result, err := admin.Purge(ctx, PurgeOptions{})
	if err != nil {
		t.Fatalf("purge with synced statements: %v (committed=%v)", err, result.Committed)
	}
	for _, table := range []string{"card_statements", "card_statement_sources", "statement_mailbox_card_links", "credit_statement_bindings", "investment_accounts", "investment_holdings"} {
		var rows int
		if err := admin.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if rows != 0 {
			t.Fatalf("%s still holds %d rows after purge", table, rows)
		}
	}
	// The encrypted mailbox credentials are personal configuration: they survive,
	// and the snapshot must show them unchanged rather than silently kept.
	for _, table := range PreservedConfigTables {
		var rows int
		if err := admin.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil { // #nosec G201 -- allow-listed table
			t.Fatal(err)
		}
		if rows != 1 {
			t.Fatalf("%s rows = %d after purge, want the credential preserved", table, rows)
		}
		if count := result.After.preservedCount(table); count.Missing {
			t.Fatalf("%s is missing from the after snapshot", table)
		}
	}
	if len(result.After.Unclassified(schemaOf(t, admin))) > 0 {
		t.Fatalf("tables still unclassified after purge: %v", result.After.Unclassified(schemaOf(t, admin)))
	}
}

// Snapshot.Unclassified is the schema-drift guard the runbook relies on: a table
// in neither list means the operator has to decide its meaning. A freshly
// migrated V2 database must not trip it.
func TestV2SchemaHasNoUnclassifiedTables(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()
	schema, err := admin.Tables(ctx)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	snap, err := admin.SnapshotNow(ctx, schema)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if unclassified := snap.Unclassified(schema.All()); len(unclassified) > 0 {
		t.Fatalf("unclassified V2 tables: %v", unclassified)
	}
}

func schemaOf(t *testing.T, admin *Admin) []string {
	t.Helper()
	schema, err := admin.Tables(context.Background())
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	return schema.All()
}

// card_statements and the two card-link tables reference accounts, and
// investment_holdings references investment_accounts. Foreign keys are never
// disabled during a purge, so the delete order is the only thing that keeps the
// run from failing halfway through the transaction.
func TestStatementTablesSortBeforeTheirParentsInDeleteOrder(t *testing.T) {
	position := map[string]int{}
	for i, table := range DeleteOrder() {
		position[table] = i
	}
	childBefore := map[string][]string{
		"card_statements":              {"accounts"},
		"card_statement_sources":       {"card_statements"},
		"statement_mailbox_card_links": {"accounts"},
		"credit_statement_bindings":    {"accounts"},
		"investment_holdings":          {"investment_accounts"},
	}
	for child, parents := range childBefore {
		childPos, ok := position[child]
		if !ok {
			t.Fatalf("%s is not in the delete order", child)
		}
		for _, parent := range parents {
			parentPos, known := position[parent]
			if !known {
				t.Fatalf("%s is not in the delete order", parent)
			}
			if childPos > parentPos {
				t.Fatalf("%s must be deleted before %s", child, parent)
			}
		}
	}
	purged := PurgeTableSet()
	inSet := map[string]bool{}
	for _, table := range purged {
		inSet[table] = true
	}
	for _, table := range PreservedConfigTables {
		if inSet[table] {
			t.Fatalf("preserved config table %s is also purged", table)
		}
	}
}
