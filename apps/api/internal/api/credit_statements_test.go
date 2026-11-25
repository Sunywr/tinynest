package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"tinynest/api/internal/db"
	"tinynest/api/internal/money"
)

func summaryMail(uid uint32, statementOn, dueOn string, amount int64) statementMailPreview {
	value := money.Minor(amount)
	return statementMailPreview{UID: uid, StatementOn: statementOn, DueOn: dueOn, AmountDue: &value}
}

// statementSyncFixture wires a QQ mailbox bound to a SPDB card and replaces the
// read-only inbox search with a scripted result, so the sync path is testable
// without IMAP.
type statementSyncFixture struct {
	t          *testing.T
	server     *httptest.Server
	client     *testClient
	database   *db.DB
	mails      []statementMailPreview
	beforeRead func()
	cardID     string
	category   string
	readsSeen  int
}

func newStatementSyncFixture(t *testing.T) *statementSyncFixture {
	t.Helper()
	fixture := &statementSyncFixture{t: t}
	ts, client, database := newTestServerWithOptions(t, Options{
		MailKey:               bytes.Repeat([]byte{7}, 32),
		StatementMailboxProbe: func(context.Context, string, string) error { return nil },
		StatementMailboxFetch: func(context.Context, string, string, string, string, string, time.Time) ([]statementMailPreview, error) {
			fixture.readsSeen++
			if fixture.beforeRead != nil {
				fixture.beforeRead()
			}
			return fixture.mails, nil
		},
	})
	t.Cleanup(func() {
		_ = database.Close()
		ts.Close()
	})
	fixture.server, fixture.client, fixture.database = ts, client, database
	fixture.category = client.firstCategory("expense")
	card := client.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "credit", "name": "浦发信用卡(4321)", "statement_day": 1, "due_day": 25,
	})
	fixture.cardID = card["id"].(string)
	client.mustOK("PUT", "/api/v1/credit-statement-mailbox",
		map[string]any{"email": "123@qq.com", "auth_code": "fixture", "version": 0})
	if status, body := client.do("PUT", fixture.bindingPath()+"?version=1", nil); status != http.StatusNoContent {
		t.Fatalf("bind card: %d %v", status, body)
	}
	return fixture
}

func (f *statementSyncFixture) bindingPath() string {
	return "/api/v1/accounts/" + f.cardID + "/credit-statement-binding"
}

// syncPath pins the mailbox configuration version so a stale read is refused.
func (f *statementSyncFixture) syncPath(version int) string {
	return "/api/v1/accounts/" + f.cardID + "/credit-statement-sync?version=" + strconv.Itoa(version)
}

func (f *statementSyncFixture) sync(version int) (int, map[string]any) {
	return f.client.do("POST", f.syncPath(version), nil)
}

func (f *statementSyncFixture) setMails(mails ...statementMailPreview) { f.mails = mails }

func (f *statementSyncFixture) statements() []map[string]any {
	f.t.Helper()
	return v012Rows(f.t, f.client.mustOK("GET", "/api/v1/credit-statements", nil))
}

func (f *statementSyncFixture) expense(amount int64, on string) {
	f.client.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": amount, "occurred_on": on,
		"category_id": f.category, "account_id": f.cardID,
	})
}

type statementCounts struct {
	statements, sources, transactions, entries, lines, audit int
}

func (f *statementSyncFixture) counts() statementCounts {
	f.t.Helper()
	var out statementCounts
	queries := []struct {
		query  string
		args   []any
		target *int
	}{
		{`SELECT COUNT(*) FROM card_statements`, nil, &out.statements},
		{`SELECT COUNT(*) FROM card_statement_sources`, nil, &out.sources},
		{`SELECT COUNT(*) FROM transactions`, nil, &out.transactions},
		{`SELECT COUNT(*) FROM journal_entries`, nil, &out.entries},
		{`SELECT COUNT(*) FROM journal_lines`, nil, &out.lines},
		{`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'sync_card_statement%'`, nil, &out.audit},
	}
	for _, item := range queries {
		if err := f.database.QueryRow(item.query, item.args...).Scan(item.target); err != nil {
			f.t.Fatalf("%s: %v", item.query, err)
		}
	}
	return out
}

func TestStatementSyncStoresBankSummaryWithoutLedgerWrites(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.expense(60000, "2026-10-20") // inside the 2026-10-02..2026-11-01 cycle
	fixture.expense(5000, "2026-11-10")  // next cycle, must not be counted
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	before := fixture.counts()
	beforePlan := number(fixture.client.mustOK("GET", "/api/v1/accounts?include=credit_debt", nil)["credit_debt"].(map[string]any)["total_debt_minor"])

	status, body := fixture.sync(1)
	if status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("sync = %d %v, want 200 with one created statement", status, body)
	}
	afterPlan := number(fixture.client.mustOK("GET", "/api/v1/accounts?include=credit_debt", nil)["credit_debt"].(map[string]any)["total_debt_minor"])
	if beforePlan != afterPlan {
		t.Fatalf("bank summary changed the ledger-derived plan: %d -> %d", beforePlan, afterPlan)
	}

	rows := fixture.statements()
	if len(rows) != 1 {
		t.Fatalf("expected one stored statement, got %v", rows)
	}
	row := rows[0]
	if row["account_id"] != fixture.cardID || row["account_name"] != "浦发信用卡(4321)" ||
		row["period_start_on"] != "2026-10-02" || row["period_end_on"] != "2026-11-01" ||
		row["due_on"] != "2026-11-25" || number(row["amount_due_minor"]) != 120000 ||
		row["currency"] != "CNY" || row["status"] != "pending_review" || number(row["source_count"]) != 1 {
		t.Fatalf("unexpected statement row: %v", row)
	}
	if number(row["ledger_charge_minor"]) != 60000 || number(row["difference_minor"]) != 60000 {
		t.Fatalf("unexpected period comparison: %v", row)
	}

	after := fixture.counts()
	if after.transactions != before.transactions || after.entries != before.entries || after.lines != before.lines {
		t.Fatalf("summary sync touched the ledger: tx %d->%d, entries %d->%d, lines %d->%d",
			before.transactions, after.transactions, before.entries, after.entries, before.lines, after.lines)
	}
	if after.statements != 1 || after.sources != 1 {
		t.Fatalf("unexpected storage: %v", after)
	}
	if after.audit != 1 {
		t.Fatalf("expected one audit row for the created statement, got %d", after.audit)
	}
}

func TestStatementSyncReplayCreatesOneStatement(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	if status, body := fixture.sync(1); status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("first sync = %d %v", status, body)
	}
	// The same message replayed, plus a re-issued mail with a new UID and the
	// same recognised numbers.
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000),
		summaryMail(12, "2026-11-01", "2026-11-25", 120000))
	status, body := fixture.sync(1)
	if status != http.StatusOK || number(body["created"]) != 0 || number(body["linked"]) != 2 {
		t.Fatalf("replay sync = %d %v, want nothing created and two linked", status, body)
	}
	counts := fixture.counts()
	if counts.statements != 1 || counts.sources != 2 {
		t.Fatalf("replay produced %d statements and %d sources, want 1 and 2", counts.statements, counts.sources)
	}
	if counts.transactions != 0 || counts.entries != 0 {
		t.Fatalf("replay wrote ledger rows: %d transactions, %d entries", counts.transactions, counts.entries)
	}
	if counts.audit != 1 {
		t.Fatalf("replay without changes must not add audit rows, got %d", counts.audit)
	}
}

func TestStatementSyncConflictKeepsRecordedAmount(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	fixture.sync(1)
	fixture.setMails(summaryMail(12, "2026-11-01", "2026-11-25", 130000))
	if status, _ := fixture.sync(1); status != http.StatusOK {
		t.Fatalf("conflicting sync = %d", status)
	}
	rows := fixture.statements()
	if len(rows) != 1 {
		t.Fatalf("expected one statement after a conflict, got %v", rows)
	}
	row := rows[0]
	if number(row["amount_due_minor"]) != 120000 || number(row["source_count"]) != 2 ||
		row["status"] != "pending_review" || row["review_reason"] == "" || row["review_reason"] == nil {
		t.Fatalf("conflict not recorded on the existing statement: %v", row)
	}
	counts := fixture.counts()
	if counts.statements != 1 || counts.transactions != 0 || counts.entries != 0 {
		t.Fatalf("conflict sync wrote extra rows: %+v", counts)
	}
}

func TestStatementSyncSkipsUnrecognisedMails(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	unreadable := statementMailPreview{UID: 7, NeedsReview: "邮件正文无法识别"}
	noAmount := statementMailPreview{UID: 8, StatementOn: "2026-11-01", NeedsReview: "请核对卡号、账单日期和人民币应还金额"}
	fixture.setMails(unreadable, noAmount)
	status, body := fixture.sync(1)
	if status != http.StatusOK || number(body["created"]) != 0 || number(body["linked"]) != 0 ||
		number(body["needs_review"]) != 2 {
		t.Fatalf("sync = %d %v, want only unrecognised mails reported", status, body)
	}
	if rows := fixture.statements(); len(rows) != 0 {
		t.Fatalf("unrecognised mails were stored: %v", rows)
	}
	counts := fixture.counts()
	if counts.statements != 0 || counts.sources != 0 || counts.audit != 0 {
		t.Fatalf("nothing should be stored, got %+v", counts)
	}
	if fixture.readsSeen != 1 {
		t.Fatalf("expected a single inbox read, got %d", fixture.readsSeen)
	}
}

func TestStatementSyncPermissionsAndStaleConfig(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))

	other := fixture.client.mustOK("POST", "/api/v1/accounts", map[string]string{"kind": "credit", "name": "浦发信用卡(9999)"})
	if status, _ := fixture.client.do("POST",
		"/api/v1/accounts/"+other["id"].(string)+"/credit-statement-sync?version=1", nil); status != http.StatusNotFound {
		t.Fatalf("unbound sync = %d, want 404", status)
	}
	if status, _ := fixture.sync(2); status != http.StatusConflict {
		t.Fatalf("stale version sync = %d, want 409", status)
	}
	if status, _ := fixture.sync(1); status != http.StatusOK {
		t.Fatalf("bound sync = %d", status)
	}
	fixture.client.mustOK("DELETE", "/api/v1/credit-statement-mailbox", map[string]any{"version": 1})
	// Disabling the mailbox releases the card binding, so the sync target is gone.
	if status, _ := fixture.sync(2); status != http.StatusNotFound {
		t.Fatalf("disabled mailbox sync = %d, want 404", status)
	}
	if counts := fixture.counts(); counts.statements != 1 {
		t.Fatalf("disabled mailbox stored %d statements, want the single earlier one", counts.statements)
	}
	if _, err := fixture.database.Exec(`UPDATE users SET role='viewer'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := fixture.sync(2); status != http.StatusForbidden {
		t.Fatalf("viewer sync = %d, want 403", status)
	}
	if rows := fixture.statements(); len(rows) != 1 {
		t.Fatalf("viewer lost read access to statements: %v", rows)
	}
	if status, body := fixture.client.doHeaders("POST", fixture.syncPath(2), nil,
		map[string]string{"X-CSRF-Token": "wrong"}); status != http.StatusForbidden {
		t.Fatalf("missing CSRF sync = %d %v", status, body)
	}
}

func TestStatementsAreHouseholdScoped(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	fixture.sync(1)
	for _, statement := range []string{
		`INSERT INTO households(id,name) VALUES('hh_other_stat','其他家庭')`,
		`INSERT INTO accounts(id,household_id,kind,name,ledger_code)
			VALUES('acc_other_stat','hh_other_stat','credit','他卡','2010:acc_other_stat')`,
		`INSERT INTO card_statements(id,household_id,account_id,period_end_on,amount_due_minor,created_at,updated_at)
			VALUES('stmt_other_stat','hh_other_stat','acc_other_stat','2026-11-01',999900,
			      '2026-11-25T00:00:00Z','2026-11-25T00:00:00Z')`,
	} {
		if _, err := fixture.database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	rows := fixture.statements()
	if len(rows) != 1 || rows[0]["account_id"] != fixture.cardID {
		t.Fatalf("household leak in statement list: %v", rows)
	}
	if _, err := fixture.database.Exec(
		`INSERT INTO card_statement_sources(id,household_id,statement_id,kind,digest,created_at)
			VALUES('src_other_stat','hh_other_stat','stmt_other_stat','email','other-digest','2026-11-25T00:00:00Z')`); err != nil {
		t.Fatalf("a household-consistent source row was rejected: %v", err)
	}
	if _, err := fixture.database.Exec(
		`INSERT INTO card_statement_sources(id,household_id,statement_id,kind,digest,created_at)
			VALUES('src_cross_home','hh_other_stat',?,'email','cross-digest','2026-11-25T00:00:00Z')`, rows[0]["id"]); err == nil {
		t.Fatal("a source row must not mix households and statements")
	}
}

func TestStatementSyncStoresUnknownCycleAsNull(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.client.mustOK("PATCH", "/api/v1/accounts/"+fixture.cardID,
		map[string]any{"statement_day": nil, "due_day": nil})
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	if status, body := fixture.sync(1); status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("sync = %d %v", status, body)
	}
	rows := fixture.statements()
	if len(rows) != 1 {
		t.Fatalf("expected one statement, got %v", rows)
	}
	row := rows[0]
	if row["period_start_on"] != nil || row["ledger_charge_minor"] != nil || row["difference_minor"] != nil {
		t.Fatalf("unconfigured cycle must stay unknown: %v", row)
	}
}

// Rotating the authorisation code while the inbox read is in flight must not
// store anything: the config version is re-verified before the write.
func TestStatementSyncRechecksMailboxBeforeWrite(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.setMails(summaryMail(11, "2026-11-01", "2026-11-25", 120000))
	fixture.beforeRead = func() {
		if _, err := fixture.database.Exec(`UPDATE statement_mailbox_configs SET version=version+1`); err != nil {
			t.Error(err)
		}
	}
	if status, _ := fixture.sync(1); status != http.StatusConflict {
		t.Fatalf("sync racing a rotated code = %d, want 409", status)
	}
	if counts := fixture.counts(); counts.statements != 0 || counts.sources != 0 || counts.audit != 0 {
		t.Fatalf("a rotated code still stored rows: %+v", counts)
	}
}
