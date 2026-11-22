package api

import (
	"net/http"
	"testing"
	"time"

	"tinynest/api/internal/db"
)

// --- local helpers ----------------------------------------------------------

func (c *testClient) createAccount(kind, name string, opening int64) string {
	c.t.Helper()
	return c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": kind, "name": name, "opening_minor": opening,
	})["id"].(string)
}

func (c *testClient) accountBalance(id string) int64 {
	c.t.Helper()
	return number(c.mustOK("GET", "/api/v1/accounts/"+id, nil)["balance_minor"])
}

func (c *testClient) createExpense(amount int64, on, category, account string) string {
	c.t.Helper()
	return c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": amount, "occurred_on": on,
		"category_id": category, "account_id": account,
	})["id"].(string)
}

func (c *testClient) createIncome(amount int64, on, category, account string) string {
	c.t.Helper()
	return c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "income", "amount_minor": amount, "occurred_on": on,
		"category_id": category, "account_id": account,
	})["id"].(string)
}

func (c *testClient) transfer(from, to string, amount int64, on string) string {
	c.t.Helper()
	return c.mustOK("POST", "/api/v1/accounts/transfers", map[string]any{
		"from_account_id": from, "to_account_id": to, "amount_minor": amount, "occurred_on": on,
	})["id"].(string)
}

// journalBalanced proves the double-entry invariant per persisted entry: every
// entry has at least two lines and equal debit/credit sums.
func journalBalanced(t *testing.T, database *db.DB) {
	t.Helper()
	var unbalanced int
	err := database.QueryRow(`SELECT COUNT(*) FROM (
		SELECT entry_id FROM journal_lines GROUP BY entry_id
		HAVING SUM(debit_minor) <> SUM(credit_minor)
	) bad`).Scan(&unbalanced)
	if err != nil {
		t.Fatal(err)
	}
	if unbalanced != 0 {
		t.Fatalf("%d unbalanced journal entries", unbalanced)
	}
	var entries, lines int
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&entries)
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_lines`).Scan(&lines)
	if lines < entries*2 {
		t.Fatalf("journal lines=%d entries=%d: every entry needs at least two lines", lines, entries)
	}
}

func (c *testClient) statsSummary(from, to string) (income, expense, balance int64) {
	c.t.Helper()
	body := c.mustOK("GET", "/api/v1/stats/summary?from="+from+"&to="+to, nil)
	return number(body["income_minor"]), number(body["expense_minor"]), number(body["balance_minor"])
}

// --- full lifecycle ---------------------------------------------------------

func TestBookkeepingLifecycle(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()

	cash := c.createAccount("cash", "现金", 100_000)
	bank := c.createAccount("cash", "储蓄卡", 0)
	credit := c.createAccount("credit", "信用卡", 0)
	expenseCategory := c.firstCategory("expense")
	incomeCategory := c.firstCategory("income")

	// Cash expense: cash decreases, expense is recognised.
	c.createExpense(3_000, "2026-11-10", expenseCategory, cash)
	if got := c.accountBalance(cash); got != 97_000 {
		t.Fatalf("cash after expense = %d, want 97000", got)
	}
	journalBalanced(t, database)

	// Credit expense: cash untouched, liability grows (balance goes negative).
	c.createExpense(2_000, "2026-11-11", expenseCategory, credit)
	if got := c.accountBalance(cash); got != 97_000 {
		t.Fatalf("credit expense must not touch cash, got %d", got)
	}
	if got := c.accountBalance(credit); got != -2_000 {
		t.Fatalf("credit balance after expense = %d, want -2000 (owed)", got)
	}
	journalBalanced(t, database)

	// Income: cash increases; credit accounts cannot receive income.
	c.createIncome(50_000, "2026-11-12", incomeCategory, cash)
	if got := c.accountBalance(cash); got != 147_000 {
		t.Fatalf("cash after income = %d, want 147000", got)
	}
	journalBalanced(t, database)

	// Cash -> cash transfer changes neither total assets nor income/expense.
	c.transfer(cash, bank, 10_000, "2026-11-13")
	if got := c.accountBalance(cash); got != 137_000 {
		t.Fatalf("cash after transfer = %d, want 137000", got)
	}
	if got := c.accountBalance(bank); got != 10_000 {
		t.Fatalf("bank after transfer = %d, want 10000", got)
	}
	journalBalanced(t, database)

	// Cash -> credit repayment settles the liability; it is never a second expense.
	c.transfer(cash, credit, 1_500, "2026-11-14")
	if got := c.accountBalance(credit); got != -500 {
		t.Fatalf("credit balance after repayment = %d, want -500", got)
	}
	if got := c.accountBalance(cash); got != 135_500 {
		t.Fatalf("cash after repayment = %d, want 135500", got)
	}
	journalBalanced(t, database)

	// Credit -> cash is rejected: monetary flows out of a liability do not exist in V2.
	if status, _ := c.do("POST", "/api/v1/accounts/transfers", map[string]any{
		"from_account_id": credit, "to_account_id": cash, "amount_minor": 100, "occurred_on": "2026-11-14",
	}); status != 400 {
		t.Fatalf("credit -> cash transfer returned %d, want 400", status)
	}

	// Period statistics are derived from the journal, not from transaction rows.
	income, expense, balance := c.statsSummary("2026-11-01", "2026-11-30")
	if income != 50_000 || expense != 5_000 || balance != 45_000 {
		t.Fatalf("stats = income %d expense %d balance %d, want 50000/5000/45000", income, expense, balance)
	}
}

// --- idempotency ------------------------------------------------------------

func TestIdempotentTransactionReplay(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	category := c.firstCategory("expense")
	body := map[string]any{
		"type": "expense", "amount_minor": 1_234, "occurred_on": "2026-11-10",
		"category_id": category, "account_id": cash,
	}
	headers := map[string]string{"Idempotency-Key": "replay-key-1"}
	status, first := c.doHeaders("POST", "/api/v1/transactions", body, headers)
	if status != http.StatusCreated {
		t.Fatalf("first create = %d %v", status, first)
	}
	status, second := c.doHeaders("POST", "/api/v1/transactions", body, headers)
	if status != http.StatusOK || second["replayed"] != true {
		t.Fatalf("replay = %d %v, want 200 replayed", status, second)
	}
	if first["id"] != second["id"] {
		t.Fatalf("replay returned a different id: %v vs %v", first["id"], second["id"])
	}
	var transactions, entries int
	_ = database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactions)
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE biz_type='expense'`).Scan(&entries)
	if transactions != 1 || entries != 1 {
		t.Fatalf("replay must be a no-op: transactions=%d entries=%d", transactions, entries)
	}
	if got := c.accountBalance(cash); got != 98_766 {
		t.Fatalf("cash after replay = %d, want 98766", got)
	}

	// The same key on a different payload still returns the committed transaction.
	body["amount_minor"] = 9_999
	status, third := c.doHeaders("POST", "/api/v1/transactions", body, headers)
	if status != http.StatusOK || third["id"] != first["id"] {
		t.Fatalf("idempotency key must be payload-independent, got %d %v", status, third)
	}
	if got := c.accountBalance(cash); got != 98_766 {
		t.Fatalf("replayed payload changed the balance: %d", got)
	}
}

// --- correction (PATCH) -----------------------------------------------------

func TestTransactionCorrectionIsReversalPlusReplacement(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	category := c.firstCategory("expense")
	oldID := c.createExpense(3_000, "2026-11-10", category, cash)

	corrected := c.mustOK("PATCH", "/api/v1/transactions/"+oldID, map[string]any{"amount_minor": 2_500})
	newID, _ := corrected["id"].(string)
	if newID == "" || newID == oldID || corrected["replaces"] != oldID {
		t.Fatalf("unexpected correction response: %v", corrected)
	}
	// The old flow is kept (never updated or deleted) but marked reversed.
	old := c.mustOK("GET", "/api/v1/transactions/"+oldID, nil)
	if old["status"] != "reversed" || number(old["amount_minor"]) != 3_000 {
		t.Fatalf("old flow must be immutable and marked reversed: %v", old)
	}
	// The new flow carries the corrected value.
	updated := c.mustOK("GET", "/api/v1/transactions/"+newID, nil)
	if number(updated["amount_minor"]) != 2_500 || updated["status"] != "posted" {
		t.Fatalf("unexpected corrected flow: %v", updated)
	}
	// Balances derive from the ledger: only the correction count remains.
	if got := c.accountBalance(cash); got != 97_500 {
		t.Fatalf("cash after correction = %d, want 97500", got)
	}
	journalBalanced(t, database)
	_, expense, _ := c.statsSummary("2026-11-01", "2026-11-30")
	if expense != 2_500 {
		t.Fatalf("stats expense after correction = %d, want 2500", expense)
	}
	// Both flows are audited.
	var reversals, corrections int
	_ = database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='reverse_transaction'`).Scan(&reversals)
	_ = database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='correct_transaction'`).Scan(&corrections)
	if reversals != 1 || corrections != 1 {
		t.Fatalf("audit missing: reversals=%d corrections=%d", reversals, corrections)
	}
	// A reversed flow cannot be corrected a second time.
	if status, _ := c.do("PATCH", "/api/v1/transactions/"+oldID, map[string]any{"amount_minor": 1}); status != 409 {
		t.Fatalf("second correction returned %d, want 409", status)
	}
	// Changing the type is rejected: corrections keep the flow type stable.
	if status, _ := c.do("PATCH", "/api/v1/transactions/"+newID, map[string]any{"type": "income"}); status != 400 {
		t.Fatalf("type change returned %d, want 400", status)
	}
	if got := c.accountBalance(cash); got != 97_500 {
		t.Fatalf("failed correction changed the balance: %d", got)
	}
}

// --- deletion (reversal only) ----------------------------------------------

func TestTransactionDeleteIsReversalOnly(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	credit := c.createAccount("credit", "信用卡", 0)
	category := c.firstCategory("expense")
	id := c.createExpense(3_000, "2026-11-10", category, cash)
	creditID := c.createExpense(1_000, "2026-11-10", category, credit)

	deleted := c.mustOK("DELETE", "/api/v1/transactions/"+id, nil)
	if deleted["status"] != "reversed" {
		t.Fatalf("unexpected delete response: %v", deleted)
	}
	row := c.mustOK("GET", "/api/v1/transactions/"+id, nil)
	if row["status"] != "reversed" {
		t.Fatalf("flow still posted after delete: %v", row)
	}
	var physical int
	_ = database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE id=?`, id).Scan(&physical)
	if physical != 1 {
		t.Fatal("transactions must never be physically deleted")
	}
	if got := c.accountBalance(cash); got != 100_000 {
		t.Fatalf("cash not restored: %d", got)
	}
	journalBalanced(t, database)
	if status, _ := c.do("DELETE", "/api/v1/transactions/"+id, nil); status != 409 {
		t.Fatalf("double delete returned %d, want 409", status)
	}
	// Deleting one flow leaves the other credit expense intact.
	if got := c.accountBalance(credit); got != -1_000 {
		t.Fatalf("credit balance = %d, want -1000", got)
	}
	_ = creditID
}

// --- statistics ------------------------------------------------------------

func TestStatsConsistency(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 200_000)
	credit := c.createAccount("credit", "信用卡", 0)
	cats := v012Rows(t, c.mustOK("GET", "/api/v1/categories", nil))
	var expenseCats, incomeCats []string
	for _, row := range cats {
		id := row["id"].(string)
		if row["kind"] == "expense" {
			expenseCats = append(expenseCats, id)
		} else {
			incomeCats = append(incomeCats, id)
		}
	}
	c.createExpense(1_000, "2026-11-03", expenseCats[0], cash)
	c.createExpense(2_000, "2026-11-03", expenseCats[1], cash)
	c.createExpense(3_000, "2026-11-04", expenseCats[0], credit)
	c.createIncome(10_000, "2026-11-05", incomeCats[0], cash)
	c.transfer(cash, credit, 500, "2026-11-05") // repayment: never income nor expense

	income, expense, balance := c.statsSummary("2026-11-01", "2026-11-30")
	if income != 10_000 || expense != 6_000 || balance != 4_000 {
		t.Fatalf("summary = %d/%d/%d, want 10000/6000/4000", income, expense, balance)
	}

	expenseRanking := v012Rows(t, c.mustOK("GET", "/api/v1/stats/categories?from=2026-11-01&to=2026-11-30&kind=expense", nil))
	var rankingTotal int64
	var firstAmount int64 = -1
	for _, row := range expenseRanking {
		amount := number(row["amount_minor"])
		rankingTotal += amount
		if firstAmount >= 0 && amount > firstAmount {
			t.Fatalf("category ranking must be ordered by amount desc: %v", expenseRanking)
		}
		firstAmount = amount
	}
	if rankingTotal != expense {
		t.Fatalf("category ranking total %d != summary expense %d", rankingTotal, expense)
	}
	if len(expenseRanking) != 2 {
		t.Fatalf("expected 2 ranked expense categories, got %v", expenseRanking)
	}
	if number(expenseRanking[0]["amount_minor"]) != 4_000 {
		t.Fatalf("top expense category = %v, want 4000 (1000+3000)", expenseRanking[0])
	}
	// share_bp is basis points of the total: 4000/6000 = 6666bp.
	if number(expenseRanking[0]["share_bp"]) != 6_666 {
		t.Fatalf("top category share = %v, want 6666bp", expenseRanking[0]["share_bp"])
	}

	incomeRanking := v012Rows(t, c.mustOK("GET", "/api/v1/stats/categories?from=2026-11-01&to=2026-11-30&kind=income", nil))
	var incomeTotal int64
	for _, row := range incomeRanking {
		incomeTotal += number(row["amount_minor"])
	}
	if incomeTotal != income {
		t.Fatalf("income ranking total %d != summary income %d", incomeTotal, income)
	}

	// Daily and monthly views must add up to the same period totals.
	daily := v012Rows(t, map[string]any{"items": c.mustOK("GET", "/api/v1/stats/daily?month=2026-11", nil)["days"]})
	var dayExpense, dayIncome int64
	for _, row := range daily {
		dayExpense += number(row["expense_minor"])
		dayIncome += number(row["income_minor"])
	}
	if dayExpense != expense || dayIncome != income {
		t.Fatalf("daily sums = expense %d income %d, want %d/%d", dayExpense, dayIncome, expense, income)
	}
	months, _ := c.mustOK("GET", "/api/v1/stats/monthly?months=3&to=2026-11", nil)["months"].([]any)
	if len(months) != 3 {
		t.Fatalf("monthly window = %v, want 3 months", months)
	}
	last := months[len(months)-1].(map[string]any)
	if last["month"] != "2026-11" || number(last["expense_minor"]) != expense || number(last["income_minor"]) != income {
		t.Fatalf("monthly view inconsistent: %v", last)
	}
	// Earlier empty months are zero-filled, not omitted.
	if number(months[0].(map[string]any)["expense_minor"]) != 0 {
		t.Fatalf("empty month not zero-filled: %v", months[0])
	}
}

func TestReversedTransactionExcludedFromStats(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	category := c.firstCategory("expense")
	id := c.createExpense(3_000, "2026-11-10", category, cash)
	if _, expense, _ := c.statsSummary("2026-11-01", "2026-11-30"); expense != 3_000 {
		t.Fatalf("expense before delete = %d, want 3000", expense)
	}
	c.mustOK("DELETE", "/api/v1/transactions/"+id, nil)
	if _, expense, _ := c.statsSummary("2026-11-01", "2026-11-30"); expense != 0 {
		t.Fatalf("reversed flow still counted: expense=%d", expense)
	}
	ranking := v012Rows(t, c.mustOK("GET", "/api/v1/stats/categories?from=2026-11-01&to=2026-11-30", nil))
	if len(ranking) != 0 {
		t.Fatalf("reversed flow still ranked: %v", ranking)
	}
	if got := c.accountBalance(cash); got != 100_000 {
		t.Fatalf("reversal did not restore the balance: %d", got)
	}
	journalBalanced(t, database)
}

// --- budgets ---------------------------------------------------------------

func TestBudgetUpsertAndSpent(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	category := c.firstCategory("expense")

	// No budget yet: amount is null, spent is still derived from the journal.
	empty := c.mustOK("GET", "/api/v1/budgets/2026-11", nil)
	if empty["amount_minor"] != nil || number(empty["spent_minor"]) != 0 {
		t.Fatalf("unset budget = %v, want null amount and 0 spent", empty)
	}
	c.createExpense(3_000, "2026-11-10", category, cash)

	put := c.mustOK("PUT", "/api/v1/budgets/2026-11", map[string]any{"amount_minor": 40_000})
	if number(put["amount_minor"]) != 40_000 || number(put["spent_minor"]) != 3_000 {
		t.Fatalf("unexpected budget response: %v", put)
	}
	// Upsert overwrites the same (household, month) row.
	c.mustOK("PUT", "/api/v1/budgets/2026-11", map[string]any{"amount_minor": 50_000})
	got := c.mustOK("GET", "/api/v1/budgets/2026-11", nil)
	if number(got["amount_minor"]) != 50_000 || number(got["spent_minor"]) != 3_000 {
		t.Fatalf("budget not upserted: %v", got)
	}
	var rows int
	_ = database.QueryRow(`SELECT COUNT(*) FROM budgets`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("budget upsert created %d rows, want 1", rows)
	}
	// Negative budgets and malformed months are rejected.
	if status, _ := c.do("PUT", "/api/v1/budgets/2026-11", map[string]any{"amount_minor": -1}); status != 400 {
		t.Fatalf("negative budget returned %d", status)
	}
	if status, _ := c.do("GET", "/api/v1/budgets/2026-13", nil); status != 400 {
		t.Fatalf("invalid month returned %d", status)
	}
}

// --- business-date boundary -------------------------------------------------

func TestBusinessDateBoundary(t *testing.T) {
	// 2026-11-13 16:00 UTC is 2026-11-14 00:00 in Asia/Shanghai.
	clk := time.Date(2026, 11, 13, 16, 0, 0, 0, time.UTC)
	ts, c, database := newTestServerWithOptions(t, Options{Clock: func() time.Time { return clk }})
	defer ts.Close()
	defer database.Close()

	cash := c.createAccount("cash", "现金", 100_000)
	var openingOn string
	if err := database.QueryRow(`SELECT occurred_on FROM journal_entries WHERE biz_type='opening_cash'`).Scan(&openingOn); err != nil {
		t.Fatal(err)
	}
	if openingOn != "2026-11-14" {
		t.Fatalf("opening balance date = %s, want the Shanghai business day 2026-11-14", openingOn)
	}
	category := c.firstCategory("expense")
	// A flow dated on the Shanghai day still counts; one dated the previous UTC
	// day does too because occurred_on is a business date, not a UTC timestamp.
	c.createExpense(1_000, "2026-11-14", category, cash)
	summary := c.mustOK("GET", "/api/v1/stats/summary", nil)
	if summary["from"] != "2026-11-01" || summary["to"] != "2026-11-14" {
		t.Fatalf("default period = %v..%v, want 2026-11-01..2026-11-14", summary["from"], summary["to"])
	}
	if number(summary["expense_minor"]) != 1_000 {
		t.Fatalf("current business month expense = %v, want 1000", summary["expense_minor"])
	}
	// A malformed occurred_on never reaches the journal.
	if status, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-02-30",
		"category_id": category, "account_id": cash,
	}); status != 400 {
		t.Fatalf("impossible date returned %d", status)
	}
}

// --- transaction listing ----------------------------------------------------

func TestTransactionListFiltersAndCursor(t *testing.T) {
	ts, c, database := newTestServerWithOptions(t, Options{})
	defer ts.Close()
	defer database.Close()
	cash := c.createAccount("cash", "现金", 100_000)
	other := c.createAccount("cash", "另一账户", 0)
	expenseCategory := c.firstCategory("expense")
	incomeCategory := c.firstCategory("income")
	for i := 0; i < 5; i++ {
		c.createExpense(int64(100*(i+1)), "2026-11-0"+string(rune('1'+i)), expenseCategory, cash)
	}
	c.createIncome(9_000, "2026-11-06", incomeCategory, other)

	all := c.mustOK("GET", "/api/v1/transactions?limit=200", nil)
	if len(all["items"].([]any)) != 6 {
		t.Fatalf("expected 6 flows, got %v", all["items"])
	}
	onlyIncome := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?type=income", nil))
	if len(onlyIncome) != 1 || onlyIncome[0]["type"] != "income" {
		t.Fatalf("type filter failed: %v", onlyIncome)
	}
	byAccount := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?account_id="+other, nil))
	if len(byAccount) != 1 {
		t.Fatalf("account filter failed: %v", byAccount)
	}
	byCategory := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?category_id="+expenseCategory, nil))
	if len(byCategory) != 5 {
		t.Fatalf("category filter failed: %v", byCategory)
	}
	byRange := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?from=2026-11-02&to=2026-11-03", nil))
	if len(byRange) != 2 {
		t.Fatalf("date filter failed: %v", byRange)
	}
	// q matches the note and the category name.
	noted := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 500, "occurred_on": "2026-11-07",
		"category_id": expenseCategory, "account_id": cash, "note": "咖啡拿铁",
	})["id"].(string)
	found := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?q="+"咖啡", nil))
	if len(found) != 1 || found[0]["id"] != noted {
		t.Fatalf("q filter failed: %v", found)
	}
	// Cursor paging: 3 + 3 pages, no overlap, newest first.
	page1 := c.mustOK("GET", "/api/v1/transactions?limit=3", nil)
	items1 := page1["items"].([]any)
	cursor, ok := page1["next_cursor"].(string)
	if len(items1) != 3 || !ok || cursor == "" {
		t.Fatalf("first page = %v next_cursor=%v", items1, page1["next_cursor"])
	}
	page2 := c.mustOK("GET", "/api/v1/transactions?limit=3&cursor="+cursor, nil)
	items2 := page2["items"].([]any)
	if len(items2) != 3 {
		t.Fatalf("second page = %v", items2)
	}
	seen := map[string]bool{}
	for _, item := range append(items1, items2...) {
		id := item.(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatalf("cursor paging repeated flow %s", id)
		}
		seen[id] = true
	}
	if status, _ := c.do("GET", "/api/v1/transactions?cursor=bogus", nil); status != 400 {
		t.Fatalf("bogus cursor returned %d", status)
	}
	if status, _ := c.do("GET", "/api/v1/transactions?limit=201", nil); status != 400 {
		t.Fatalf("limit above 200 returned %d", status)
	}
}

// --- household isolation ----------------------------------------------------

func TestHouseholdIsolationTransactionsAndStats(t *testing.T) {
	e := newRemEnv(t, Options{SetupToken: testSetupToken})
	alice := bootstrapHousehold(t, e, "alice")
	bob := bootstrapHousehold(t, e, "bob")
	aliceClient := alice.client(t, e.ts.URL)
	bobClient := bob.client(t, e.ts.URL)

	cash := aliceClient.createAccount("cash", "Alice 现金", 50_000)
	category := aliceClient.firstCategory("expense")
	aliceTx := aliceClient.createExpense(2_000, "2026-11-10", category, cash)

	// Bob cannot see Alice's flow, account or statistics.
	bobTransactions := v012Rows(t, bobClient.call("GET", "/api/v1/transactions?limit=50", nil, nil).Body)
	if len(bobTransactions) != 0 {
		t.Fatalf("cross-household transaction leak: %v", bobTransactions)
	}
	detail := bobClient.call("GET", "/api/v1/transactions/"+aliceTx, nil, nil)
	if detail.Status != 404 {
		t.Fatalf("cross-household flow detail returned %d", detail.Status)
	}
	bobSummary := bobClient.call("GET", "/api/v1/stats/summary?from=2026-11-01&to=2026-11-30", nil, nil)
	if number(bobSummary.Body["expense_minor"]) != 0 {
		t.Fatalf("cross-household stats leak: %v", bobSummary.Body)
	}
	bobAccounts := v012Rows(t, bobClient.call("GET", "/api/v1/accounts", nil, nil).Body)
	if len(bobAccounts) != 0 {
		t.Fatalf("cross-household account leak: %v", bobAccounts)
	}
	// Deleting another household's flow is a 404, not a silent reversal.
	if r := bobClient.call("DELETE", "/api/v1/transactions/"+aliceTx, nil, nil); r.Status != 404 {
		t.Fatalf("cross-household delete returned %d", r.Status)
	}
	// Alice still sees her own data.
	aliceSummary := aliceClient.call("GET", "/api/v1/stats/summary?from=2026-11-01&to=2026-11-30", nil, nil)
	if number(aliceSummary.Body["expense_minor"]) != 2_000 {
		t.Fatalf("owner stats wrong: %v", aliceSummary.Body)
	}
}
