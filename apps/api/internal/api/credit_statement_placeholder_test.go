package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// The placeholder path serves the household that lets the bank e-mail arrive but
// never uploads the statement details: one 待补明细 expense carries the amount
// the bank reported as due, dated at the repayment deadline, so the repayment
// plan can reserve the cash before the money leaves. Amounts and dates always
// come from the stored bank summary, never from the request body.

func (f *statementSyncFixture) placeholderPath() string {
	return "/api/v1/accounts/" + f.cardID + "/credit-statement-placeholder"
}

// postedStatement syncs one bank summary and returns the stored row.
func (f *statementSyncFixture) postedStatement(t *testing.T, statementOn, dueOn string, amount int64) map[string]any {
	t.Helper()
	f.setMails(summaryMail(9001, statementOn, dueOn, amount))
	if status, body := f.sync(1); status != http.StatusOK || number(body["created"]) != 1 {
		t.Fatalf("sync = %d %v, want one statement created", status, body)
	}
	rows := f.statements()
	if len(rows) != 1 {
		t.Fatalf("expected one stored statement, got %v", rows)
	}
	return rows[0]
}

func (f *statementSyncFixture) placeholder(statementID any) (int, map[string]any) {
	return f.client.do("POST", f.placeholderPath(), map[string]any{"statement_id": statementID})
}

func relativeDate(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

func placeholderError(body map[string]any) string {
	err, _ := body["error"].(map[string]any)
	if err == nil {
		return ""
	}
	return fmt.Sprint(err["code"])
}

func (f *statementSyncFixture) plan() map[string]any {
	f.t.Helper()
	body := f.client.mustOK("GET", "/api/v1/accounts?include=credit_debt", nil)
	if body["credit_debt"] == nil {
		f.t.Fatalf("no credit_debt projection: %v", body)
	}
	return body["credit_debt"].(map[string]any)
}

func (f *statementSyncFixture) planCard() map[string]any {
	f.t.Helper()
	raw, _ := f.plan()["cards"].([]any)
	for _, value := range raw {
		card, ok := value.(map[string]any)
		if !ok {
			f.t.Fatalf("plan card is not an object: %v", value)
		}
		if card["account_id"] == f.cardID {
			return card
		}
	}
	f.t.Fatalf("card %s missing from the plan: %v", f.cardID, raw)
	return nil
}

// categoryStats lists the expense categories with a non-zero net for a period;
// a fully reversed category disappears because it nets to zero.
func (f *statementSyncFixture) categoryStats(from, to string) []map[string]any {
	f.t.Helper()
	return v012Rows(f.t, f.client.mustOK("GET",
		"/api/v1/stats/categories?from="+from+"&to="+to, nil))
}

func TestStatementPlaceholderWritesAwaitingDetailTransaction(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	before := fixture.counts()
	if card := fixture.planCard(); number(card["total_debt_minor"]) != 0 {
		t.Fatalf("fixture starts in debt: %v", card)
	}

	status, body := fixture.placeholder(statement["id"])
	if status != http.StatusCreated {
		t.Fatalf("placeholder = %d %v, want 201", status, body)
	}
	transactionID, _ := body["transaction_id"].(string)
	if transactionID == "" {
		t.Fatalf("placeholder returned no transaction id: %v", body)
	}
	if number(body["amount_minor"]) != 120000 || body["occurred_on"] != "2026-11-25" ||
		body["period_end_on"] != "2026-11-01" || body["due_on"] != "2026-11-25" ||
		body["account_id"] != fixture.cardID {
		t.Fatalf("placeholder echoes the wrong bank figures: %v", body)
	}

	// The ledger row is an ordinary expense on the card, so every balance and
	// report still rebuilds from entries: 借 待补明细 / 贷 信用卡欠款.
	row := fixture.client.mustOK("GET", "/api/v1/transactions/"+transactionID, nil)
	if row["type"] != "expense" || number(row["amount_minor"]) != 120000 ||
		row["occurred_on"] != "2026-11-25" || row["account_id"] != fixture.cardID ||
		row["status"] != "posted" {
		t.Fatalf("placeholder transaction: %v", row)
	}
	if name := fmt.Sprint(row["category_name"]); name != "待补明细" {
		t.Fatalf("placeholder must sit in its own 待补明细 category, got %q", name)
	}

	after := fixture.counts()
	if after.transactions != before.transactions+1 || after.entries != before.entries+1 ||
		after.lines != before.lines+2 || after.placeholders != 1 {
		t.Fatalf("placeholder writes = tx %d->%d, entries %d->%d, lines %d->%d, links %d",
			before.transactions, after.transactions, before.entries, after.entries,
			before.lines, after.lines, after.placeholders)
	}
	if after.audit != before.audit+1 {
		t.Fatalf("expected one audit row, got %d -> %d", before.audit, after.audit)
	}

	// The point of the placeholder: the plan reserves the money, and it schedules
	// it against the bank's own cycle rather than the cycle the deadline date
	// happens to fall in. This bill closed 2026-11-01 and was due 2026-11-25, so
	// the amount is overdue, never unbilled.
	card := fixture.planCard()
	if number(card["total_debt_minor"]) != 120000 || number(card["overdue_minor"]) != 120000 ||
		number(card["unbilled_minor"]) != 0 {
		t.Fatalf("plan did not reserve the closed bill: %v", card)
	}
	if total := number(fixture.plan()["total_debt_minor"]); total != 120000 {
		t.Fatalf("household plan total = %d, want 120000", total)
	}
	found := false
	for _, item := range fixture.categoryStats("2026-11-01", "2026-11-30") {
		if item["name"] == "待补明细" {
			found = true
			if number(item["amount_minor"]) != 120000 {
				t.Fatalf("placeholder spending counted wrong: %v", item)
			}
		}
	}
	if !found {
		t.Fatal("the placeholder is missing from the November category stats")
	}
}

func TestStatementPlaceholderCountsFutureDeadline(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	// A card without cycle configuration has no grid to schedule against, so only
	// the stored statement dates can carry the deadline, and a deadline that has
	// not arrived yet must still be reserved before it is due.
	fixture.client.mustOK("PATCH", "/api/v1/accounts/"+fixture.cardID,
		map[string]any{"statement_day": nil, "due_day": nil})
	statement := fixture.postedStatement(t, relativeDate(-4), relativeDate(+4), 88000)
	if card := fixture.planCard(); number(card["total_debt_minor"]) != 0 {
		t.Fatalf("fixture starts in debt: %v", card)
	}

	if status, body := fixture.placeholder(statement["id"]); status != http.StatusCreated {
		t.Fatalf("placeholder = %d %v, want 201", status, body)
	}
	if card := fixture.planCard(); number(card["total_debt_minor"]) != 88000 {
		t.Fatalf("future deadline not reserved: %v", card)
	}
}

func TestStatementPlaceholderReversalNetsThePeriodToZero(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	_, body := fixture.placeholder(statement["id"])
	transactionID := body["transaction_id"].(string)

	// Details arriving later replace the placeholder. Deleting it writes the
	// reversal, which reuses the original business date, so the reserved debt
	// clears and the 待补明细 category nets back to nothing in the same period.
	fixture.client.mustOK("DELETE", "/api/v1/transactions/"+transactionID, map[string]any{})

	card := fixture.planCard()
	if number(card["total_debt_minor"]) != 0 || number(card["overdue_minor"]) != 0 {
		t.Fatalf("reversal left debt behind: %v", card)
	}
	for _, item := range fixture.categoryStats("2026-11-01", "2026-11-30") {
		if item["name"] == "待补明细" {
			t.Fatalf("reversed placeholder still counted as spending: %v", item)
		}
	}
	if row := fixture.statements()[0]; fmt.Sprint(row["placeholder_state"]) != "reversed" {
		t.Fatalf("statement still reports a live placeholder: %v", row)
	}
}

func TestStatementPlaceholderReplayReturnsExistingTransaction(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	_, first := fixture.placeholder(statement["id"])
	before := fixture.counts()

	status, body := fixture.placeholder(statement["id"])
	if status != http.StatusConflict || placeholderError(body) != "placeholder_exists" {
		t.Fatalf("replay = %d %v, want 409 placeholder_exists", status, body)
	}
	if body["transaction_id"] != first["transaction_id"] {
		t.Fatalf("replay pointed at another transaction: %v vs %v", body, first)
	}
	if after := fixture.counts(); after != before {
		t.Fatalf("replay wrote rows: %+v -> %+v", before, after)
	}
}

func TestStatementPlaceholderRefusedWhenCycleAlreadyRecorded(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	fixture.expense(60000, "2026-10-20") // inside the 2026-10-02..2026-11-01 cycle
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	before := fixture.counts()

	status, body := fixture.placeholder(statement["id"])
	if status != http.StatusConflict || placeholderError(body) != "cycle_already_recorded" {
		t.Fatalf("recorded cycle = %d %v, want 409 cycle_already_recorded", status, body)
	}
	if after := fixture.counts(); after != before {
		t.Fatalf("refused placeholder wrote rows: %+v -> %+v", before, after)
	}
	if card := fixture.planCard(); number(card["total_debt_minor"]) != 60000 {
		t.Fatalf("existing charges changed: %v", card)
	}
}

func TestStatementPlaceholderGuardsStatementState(t *testing.T) {
	cases := []struct {
		name      string
		statement string
		due       string
		amount    int64
		code      string
	}{
		{"unclosed cycle", relativeDate(3), relativeDate(33), 120000, "cycle_not_closed"},
		{"zero amount", relativeDate(-30), relativeDate(-10), 0, "amount_unknown"},
		{"unknown deadline", relativeDate(-30), "", 120000, "due_date_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newStatementSyncFixture(t)
			// Without a cycle grid nothing can be inferred about the deadline, and
			// the bank e-mail did not report one either.
			fixture.client.mustOK("PATCH", "/api/v1/accounts/"+fixture.cardID,
				map[string]any{"statement_day": nil, "due_day": nil})
			fixture.setMails(summaryMail(9100, tc.statement, tc.due, tc.amount))
			if status, body := fixture.sync(1); status != http.StatusOK {
				t.Fatalf("sync = %d %v", status, body)
			}
			var statementID any
			for _, row := range fixture.statements() {
				if row["period_end_on"] == tc.statement {
					statementID = row["id"]
				}
			}
			if statementID == nil {
				t.Fatalf("statement %v was not stored: %v", tc, fixture.statements())
			}
			status, body := fixture.placeholder(statementID)
			if status != http.StatusConflict || placeholderError(body) != tc.code {
				t.Fatalf("%s = %d %v, want 409 %s", tc.name, status, body, tc.code)
			}
			if counts := fixture.counts(); counts.entries != 0 || counts.transactions != 0 ||
				counts.placeholders != 0 {
				t.Fatalf("%s still wrote rows: %+v", tc.name, counts)
			}
		})
	}
}

func TestStatementPlaceholderDerivesDeadlineFromCardConfig(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	// The bank reported a closing date but no repayment date: fall back to the
	// card's own due_day so the placeholder still lands on the real deadline.
	statement := fixture.postedStatement(t, "2026-11-01", "", 120000)
	if statement["due_on"] != nil {
		t.Fatalf("fixture expected no reported deadline: %v", statement)
	}
	if status, body := fixture.placeholder(statement["id"]); status != http.StatusCreated {
		t.Fatalf("placeholder = %d %v, want 201", status, body)
	} else if body["due_on"] != "2026-11-25" || body["occurred_on"] != "2026-11-25" {
		t.Fatalf("deadline not derived from due_day: %v", body)
	}
}

func TestStatementPlaceholderPermissionsAndScoping(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	statementID := statement["id"].(string)

	for _, statement := range []string{
		`INSERT INTO households(id,name) VALUES('hh_other_ph','其他家庭')`,
		`INSERT INTO accounts(id,household_id,kind,name,ledger_code)
			VALUES('acc_other_ph','hh_other_ph','credit','他卡','2010:acc_other_ph')`,
		`INSERT INTO card_statements(id,household_id,account_id,period_end_on,due_on,amount_due_minor,created_at,updated_at)
			VALUES('stmt_other_ph','hh_other_ph','acc_other_ph','2026-11-01','2026-11-25',500000,
			      '2026-11-25T00:00:00Z','2026-11-25T00:00:00Z')`,
	} {
		if _, err := fixture.database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	if status, _ := fixture.placeholder("stmt_other_ph"); status != http.StatusNotFound {
		t.Fatalf("other household statement = %d, want 404", status)
	}
	if status, _ := fixture.placeholder("stmt_missing_ph"); status != http.StatusNotFound {
		t.Fatalf("unknown statement = %d, want 404", status)
	}
	if status, _ := fixture.client.doHeaders("POST", fixture.placeholderPath(),
		map[string]any{"statement_id": statementID},
		map[string]string{"X-CSRF-Token": "wrong"}); status != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d, want 403", status)
	}
	if _, err := fixture.database.Exec(`UPDATE users SET role='viewer'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := fixture.placeholder(statementID); status != http.StatusForbidden {
		t.Fatalf("viewer = %d, want 403", status)
	}
	if counts := fixture.counts(); counts.transactions != 0 || counts.entries != 0 ||
		counts.placeholders != 0 {
		t.Fatalf("refused requests wrote rows: %+v", counts)
	}
}

func TestStatementPlaceholderListedWithStatement(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	if row := fixture.statements()[0]; row["placeholder_transaction_id"] != nil {
		t.Fatalf("statement reports a placeholder before it exists: %v", row)
	}
	_, body := fixture.placeholder(statement["id"])

	row := fixture.statements()[0]
	if row["placeholder_transaction_id"] != body["transaction_id"] {
		t.Fatalf("statement does not report the placeholder: %v vs %v", row, body)
	}
	if state := fmt.Sprint(row["placeholder_state"]); state != "awaiting_detail" {
		t.Fatalf("placeholder state = %q, want awaiting_detail", state)
	}
	if number(row["amount_due_minor"]) != 120000 {
		t.Fatalf("statement amount changed: %v", row)
	}
}

func TestStatementPlaceholderRowsAreHouseholdConsistent(t *testing.T) {
	fixture := newStatementSyncFixture(t)
	statement := fixture.postedStatement(t, "2026-11-01", "2026-11-25", 120000)
	_, body := fixture.placeholder(statement["id"])

	// A link row must never mix households: it is keyed by the household that owns
	// the bank statement.
	_, err := fixture.database.Exec(
		`INSERT INTO statement_placeholders(household_id,statement_id,transaction_id,period_start_on,period_end_on,due_on,created_at)
			VALUES('hh_other_ph',?,?,NULL,?,?,?)`, statement["id"], body["transaction_id"],
		"2026-11-01", "2026-11-25", "2026-11-25T00:00:00Z")
	if err == nil {
		t.Fatal("a placeholder row linked another household to this statement")
	}
	var linkHousehold, txHousehold string
	if err := fixture.database.QueryRow(
		`SELECT household_id FROM statement_placeholders WHERE statement_id=?`,
		statement["id"]).Scan(&linkHousehold); err != nil {
		t.Fatalf("placeholder link row missing: %v", err)
	}
	if err := fixture.database.QueryRow(
		`SELECT household_id FROM transactions WHERE id=?`, body["transaction_id"]).Scan(&txHousehold); err != nil {
		t.Fatalf("placeholder transaction missing: %v", err)
	}
	if linkHousehold != txHousehold || linkHousehold == "hh_other_ph" {
		t.Fatalf("placeholder rows disagree on household: %q vs %q", linkHousehold, txHousehold)
	}
}
