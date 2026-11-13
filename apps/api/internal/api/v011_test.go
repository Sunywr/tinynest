package api

import (
	"testing"
	"time"

	"tinynest/api/internal/credit"
	"tinynest/api/internal/db"
)

// findRow returns the first item of a {"items":[...]} response whose key equals value.
func findRow(t *testing.T, out map[string]any, key, value string) map[string]any {
	t.Helper()
	items, _ := out["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m[key] == value {
			return m
		}
	}
	t.Fatalf("row %s=%s not found in %v", key, value, out)
	return nil
}

// T1: full credit lifecycle - spend / limit / partial repayment / refund / settle.
// Every assertion is against real HTTP + real SQLite, never a mock.
func TestV011_T1_CreditStatementLifecycle(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "T1 工资卡", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	expCat := c.firstCategory("expense")
	ci := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "T1 信用卡", "credit_limit_minor": 2_000_000,
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)

	creditTx := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100_000, "occurred_on": "2026-11-12",
		"payment_kind": "credit", "credit_instrument_id": ci,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 100_000}},
	})["id"].(string)

	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 1_000_000 {
		t.Fatalf("credit expense must not touch cash, got %d", got)
	}
	inst := findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if number(inst["outstanding_minor"]) != 100_000 || number(inst["used_minor"]) != 100_000 ||
		number(inst["available_minor"]) != 1_900_000 || number(inst["usage_bps"]) != 500 {
		t.Fatalf("limit after spend wrong: %v", inst)
	}
	if inst["credit_limit_configured"] != true {
		t.Fatalf("limit must be reported as configured: %v", inst)
	}

	_, stmtOut := c.do("GET", "/api/v1/credit-statements", nil)
	items, _ := stmtOut["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 statement, got %d", len(items))
	}
	stmt := items[0].(map[string]any)
	stmtID := stmt["id"].(string)
	if number(stmt["system_due_minor"]) != 100_000 {
		t.Fatalf("statement due = %v", stmt["system_due_minor"])
	}
	if _, ok := stmt["derived_status"]; !ok {
		t.Fatalf("statement must expose date/amount derived status: %v", stmt)
	}

	// Detail endpoint: items + reconciliation diff + derived status.
	detail := c.mustOK("GET", "/api/v1/credit-statements/"+stmtID, nil)
	if number(detail["remaining_minor"]) != 100_000 {
		t.Fatalf("detail remaining = %v", detail["remaining_minor"])
	}
	if _, ok := detail["items"].([]any); !ok {
		t.Fatalf("detail must include statement items: %v", detail)
	}

	// Partial repayment 600.00 -> cash -60000, liability -60000, expense unchanged.
	c.mustOK("POST", "/api/v1/credit-statements/"+stmtID+"/payments", map[string]any{
		"amount_minor": 60_000, "paid_on": "2026-11-12", "from_account_id": cash,
	})
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 940_000 {
		t.Errorf("cash after repayment = %d, want 940000", got)
	}
	inst = findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if number(inst["available_minor"]) != 1_960_000 {
		t.Errorf("available after repayment = %v", inst["available_minor"])
	}

	// Refund 300.00 back to the card reduces both expense and liability.
	c.mustOK("POST", "/api/v1/refunds", map[string]any{
		"transaction_id": creditTx, "amount_minor": 30_000, "refund_on": "2026-11-13",
		"destination_type": "credit", "destination_id": ci,
	})
	ref := c.mustOK("GET", "/api/v1/refunds/by-transaction/"+creditTx, nil)
	if number(ref["net_expense_minor"]) != 70_000 {
		t.Errorf("net expense after refund = %v", ref["net_expense_minor"])
	}
	inst = findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if number(inst["available_minor"]) != 1_990_000 {
		t.Errorf("available after refund = %v", inst["available_minor"])
	}

	// Settle the remaining 100.00 with an idempotency key: the replay must not
	// double-book cash or the credit liability.
	detail = c.mustOK("GET", "/api/v1/credit-statements/"+stmtID, nil)
	if number(detail["remaining_minor"]) != 10_000 {
		t.Fatalf("remaining before final payment = %v", detail["remaining_minor"])
	}
	c.headers = map[string]string{"Idempotency-Key": "t1-repay-1"}
	first := c.mustOK("POST", "/api/v1/credit-statements/"+stmtID+"/payments", map[string]any{
		"amount_minor": 10_000, "paid_on": "2026-11-14", "from_account_id": cash,
	})
	second := c.mustOK("POST", "/api/v1/credit-statements/"+stmtID+"/payments", map[string]any{
		"amount_minor": 10_000, "paid_on": "2026-11-14", "from_account_id": cash,
	})
	c.headers = nil
	if first["id"] != second["id"] || second["replayed"] != true {
		t.Fatalf("idempotent replay broken: first=%v second=%v", first, second)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 930_000 {
		t.Errorf("replay double-booked cash: %d", got)
	}
	inst = findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if number(inst["outstanding_minor"]) != 0 || number(inst["available_minor"]) != 2_000_000 {
		t.Errorf("liability must be zero after settlement: %v", inst)
	}
	detail = c.mustOK("GET", "/api/v1/credit-statements/"+stmtID, nil)
	if detail["derived_status"] != "paid" {
		t.Errorf("derived status after settlement = %v", detail["derived_status"])
	}
	// No expense may be booked twice.
	summary := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	if number(summary["expense_minor"]) != 70_000 {
		t.Errorf("expense must be 70000 (100000-30000), got %v", summary["expense_minor"])
	}
}

// Status must be derived from facts + dates, never a stale cached column.
func TestV011_CreditStatementStatusDerivation(t *testing.T) {
	cases := []struct {
		name, end, due, today string
		sysDue, paid          int64
		want                  string
	}{
		{"unbilled", "2026-11-30", "2026-12-10", "2026-11-13", 100_000, 0, "unbilled"},
		{"billed", "2026-11-30", "2026-12-10", "2026-12-01", 100_000, 0, "billed"},
		{"partial", "2026-11-30", "2026-12-10", "2026-12-05", 100_000, 40_000, "partial"},
		{"paid", "2026-11-30", "2026-12-10", "2026-12-05", 100_000, 100_000, "paid"},
		{"overdue", "2026-11-30", "2026-12-10", "2026-12-11", 100_000, 40_000, "overdue"},
		{"overdue_unpaid", "2026-11-30", "2026-12-10", "2026-12-11", 100_000, 0, "overdue"},
		{"empty_cycle_unbilled", "2026-11-30", "2026-12-10", "2026-11-13", 0, 0, "unbilled"},
		{"empty_cycle_billed", "2026-11-30", "2026-12-10", "2026-12-01", 0, 0, "billed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveStatementStatus(tc.end, tc.due, tc.today, tc.sysDue, tc.paid); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

// Month-end / leap-year / year-boundary rules are already in the kernel; M1
// pins them with explicit new boundary assertions so the UI can trust them.
func TestV011_CycleBoundaries(t *testing.T) {
	cases := []struct {
		anchor                      string
		start, end, due             int
		offset                      int
		wantStart, wantEnd, wantDue string
	}{
		{"2026-02-13", 1, 31, 10, 1, "2026-02-01", "2026-02-28", "2026-03-10"},
		{"2028-02-20", 1, 31, 10, 1, "2028-02-01", "2028-02-29", "2028-03-10"},
		{"2026-04-15", 1, 31, 31, 1, "2026-04-01", "2026-04-30", "2026-05-31"},
		{"2026-12-26", 26, 25, 10, 1, "2026-12-26", "2027-01-25", "2027-02-10"},
	}
	for _, tc := range cases {
		anchor, err := time.Parse("2006-01-02", tc.anchor)
		if err != nil {
			t.Fatal(err)
		}
		cy, err := credit.CycleFor(anchor, tc.start, tc.end, tc.due, tc.offset)
		if err != nil {
			t.Fatalf("%s: %v", tc.anchor, err)
		}
		if cy.Start.Format("2006-01-02") != tc.wantStart || cy.End.Format("2006-01-02") != tc.wantEnd || cy.Due.Format("2006-01-02") != tc.wantDue {
			t.Errorf("%s: got %s ~ %s due %s, want %s ~ %s due %s", tc.anchor,
				cy.Start.Format("2006-01-02"), cy.End.Format("2006-01-02"), cy.Due.Format("2006-01-02"),
				tc.wantStart, tc.wantEnd, tc.wantDue)
		}
	}
}

// Editing an instrument or manually adjusting one statement's dates must not
// silently recompute an already-issued statement's amounts or journal lines.
func TestV011_CreditInstrumentPatchAndStatementAdjust(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	expCat := c.firstCategory("expense")
	ci := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "可编辑卡", "cycle_start_day": 1,
		"cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 50_000, "occurred_on": "2026-11-12",
		"payment_kind": "credit", "credit_instrument_id": ci,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 50_000}},
	})
	stmtID := findRow(t, c.mustOK("GET", "/api/v1/credit-statements", nil), "instrument_id", ci)["id"].(string)

	// No limit configured -> never invent a percentage.
	inst := findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if inst["credit_limit_configured"] != false || inst["limit_display"] != "未配置" {
		t.Fatalf("unknown limit must be reported as 未配置: %v", inst)
	}

	patched := c.mustOK("PATCH", "/api/v1/credit-instruments/"+ci, map[string]any{
		"credit_limit_minor": 500_000, "due_day": 15, "reason": "银行调整还款日",
	})
	if patched["status"] != "active" {
		t.Fatalf("patch response: %v", patched)
	}
	inst = findRow(t, c.mustOK("GET", "/api/v1/credit-instruments", nil), "id", ci)
	if number(inst["due_day"]) != 15 || number(inst["usage_bps"]) != 1000 {
		t.Fatalf("patch not applied: %v", inst)
	}
	// Historical statement keeps its original due date and amount.
	detail := c.mustOK("GET", "/api/v1/credit-statements/"+stmtID, nil)
	if detail["due_on"] != "2026-12-10" || number(detail["system_due_minor"]) != 50_000 {
		t.Fatalf("historical statement was silently recomputed: %v", detail)
	}

	// Explicit single-period adjustment, with reason + audit trail.
	adj := c.mustOK("PATCH", "/api/v1/credit-statements/"+stmtID+"/cycle", map[string]any{
		"due_on": "2026-12-20", "reason": "平台延期",
	})
	if adj["status"] != "adjusted" {
		t.Fatalf("adjust response: %v", adj)
	}
	detail = c.mustOK("GET", "/api/v1/credit-statements/"+stmtID, nil)
	if detail["due_on"] != "2026-12-20" || number(detail["system_due_minor"]) != 50_000 {
		t.Fatalf("adjustment must change dates only, never amounts: %v", detail)
	}
	if number(detail["adjustment_count"]) != 1 {
		t.Fatalf("adjustment must be audited: %v", detail)
	}

	// Deactivate: further credit spending on this instrument is refused.
	c.mustOK("PATCH", "/api/v1/credit-instruments/"+ci, map[string]any{"status": "inactive"})
	code, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10_000, "occurred_on": "2026-11-13",
		"payment_kind": "credit", "credit_instrument_id": ci,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 10_000}},
	})
	if code == 201 || code == 200 {
		t.Fatal("inactive credit instrument must not accept new spending")
	}
}

func subjectMinor(t *testing.T, database *db.DB, code string) int64 {
	t.Helper()
	var v int64
	if err := database.QueryRow(`SELECT COALESCE(SUM(l.debit_minor-l.credit_minor),0)
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id WHERE l.account_code=?`, code).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func makeDebt(t *testing.T, c *testClient, kind string, principal int64, startOn string) string {
	t.Helper()
	return c.mustOK("POST", "/api/v1/debts", map[string]any{
		"kind": kind, "name": "T2 " + kind, "principal_minor": principal,
		"start_on": startOn, "due_day": 10, "cycle": "monthly",
	})["id"].(string)
}

func makeDebtSchedule(t *testing.T, c *testClient, id string, items ...map[string]any) {
	t.Helper()
	list := make([]any, 0, len(items))
	for _, it := range items {
		list = append(list, it)
	}
	c.mustOK("POST", "/api/v1/debts/"+id+"/schedule", map[string]any{"items": list})
}

// T2: paying a mortgage instalment with a credit card must reduce the loan
// principal, expense only the interest, and create (not consume) cash only when
// the credit card itself is later repaid. No double counting anywhere.
func TestV011_T2_CreditCardPaysMortgage(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "T2 工资卡", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	ci := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "T2 信用卡", "credit_limit_minor": 2_000_000,
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)
	debt := makeDebt(t, c, "mortgage", 1_000_000, "2026-11-01")
	makeDebtSchedule(t, c, debt, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 150_000, "interest_minor": 50_000, "fee_minor": 0,
	})

	pay := c.mustOK("POST", "/api/v1/debts/"+debt+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 200_000, "paid_on": "2026-12-10",
		"channel": "credit", "channel_id": ci,
		"principal_minor": 150_000, "interest_minor": 50_000, "fee_minor": 0,
	})
	if pay["status"] != "confirmed" {
		t.Fatalf("payment not confirmed: %v", pay)
	}
	if got := subjectMinor(t, database, "2030:"+debt); got != -850_000 {
		t.Errorf("loan principal liability = %d, want -850000", got)
	}
	if got := subjectMinor(t, database, "5100"); got != 50_000 {
		t.Errorf("interest expense = %d, want 50000", got)
	}
	if got := subjectMinor(t, database, "2010:"+ci); got != -200_000 {
		t.Errorf("credit liability = %d, want -200000", got)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 1_000_000 {
		t.Errorf("cash must be untouched by a credit-channel payment, got %d", got)
	}

	// The card repayment settles ONLY the credit liability - never re-books the
	// mortgage principal or the interest expense.
	stmt := findRow(t, c.mustOK("GET", "/api/v1/credit-statements", nil), "instrument_id", ci)["id"].(string)
	c.mustOK("POST", "/api/v1/credit-statements/"+stmt+"/payments", map[string]any{
		"amount_minor": 200_000, "paid_on": "2026-12-11", "from_account_id": cash,
	})
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 800_000 {
		t.Errorf("cash after card repayment = %d, want 800000", got)
	}
	if got := subjectMinor(t, database, "2010:"+ci); got != 0 {
		t.Errorf("credit liability after repayment = %d, want 0", got)
	}
	if got := subjectMinor(t, database, "2030:"+debt); got != -850_000 {
		t.Errorf("mortgage principal was re-booked = %d", got)
	}
	if got := subjectMinor(t, database, "5100"); got != 50_000 {
		t.Errorf("interest was double counted = %d", got)
	}
	sum := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-12-01&to=2026-12-31", nil)
	if number(sum["expense_minor"]) != 50_000 {
		t.Errorf("December expense must be 50000 (interest only), got %v", sum["expense_minor"])
	}
}

func TestV011_DebtContractCRUDAndValidation(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// Insurance must never fabricate a loan principal liability.
	code, _ := c.do("POST", "/api/v1/debts", map[string]any{
		"kind": "insurance", "name": "违规保险", "principal_minor": 10_000, "start_on": "2026-11-01",
	})
	if code == 201 || code == 200 {
		t.Fatal("insurance with principal must be rejected")
	}

	loan := makeDebt(t, c, "loan", 300_000, "2026-11-01")
	insurance := makeDebt(t, c, "insurance", 0, "2026-11-01")
	insDetail := c.mustOK("GET", "/api/v1/debts/"+insurance, nil)
	if insDetail["ledger_code"] != nil {
		t.Fatalf("insurance must have no loan liability subject: %v", insDetail["ledger_code"])
	}
	// Opening loan liability is booked against opening equity, never income.
	if got := subjectMinor(t, database, "2030:"+loan); got != -300_000 {
		t.Errorf("opening loan liability = %d, want -300000", got)
	}
	if got := subjectMinor(t, database, "3010"); got != 300_000 {
		t.Errorf("opening liability must debit opening equity, got %d", got)
	}

	makeDebtSchedule(t, c, loan, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 100_000, "interest_minor": 10_000, "fee_minor": 1_000,
	})
	// Insurance premium is a fee, never a principal.
	makeDebtSchedule(t, c, insurance, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 0, "interest_minor": 0, "fee_minor": 12_000,
	})
	code, _ = c.do("POST", "/api/v1/debts/"+insurance+"/schedule", map[string]any{
		"items": []any{map[string]any{
			"installment_no": 2, "due_on": "2027-01-10",
			"principal_minor": 5_000, "interest_minor": 0, "fee_minor": 0,
		}},
	})
	if code == 201 || code == 200 {
		t.Fatal("insurance schedule with principal must be rejected")
	}

	detail := c.mustOK("GET", "/api/v1/debts/"+loan, nil)
	sched, _ := detail["schedule"].([]any)
	if len(sched) != 1 {
		t.Fatalf("schedule items = %d, want 1", len(sched))
	}
	first := sched[0].(map[string]any)
	if number(first["total_minor"]) != 111_000 {
		t.Errorf("total must equal principal+interest+fee, got %v", first["total_minor"])
	}

	// Explicit split is mandatory: no automatic allocation policy is assumed.
	code, _ = c.do("POST", "/api/v1/debts/"+loan+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 50_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cashIDForTest(t, c),
	})
	if code == 201 || code == 200 {
		t.Fatal("payment without an explicit principal/interest/fee split must be rejected")
	}

	listed := c.mustOK("GET", "/api/v1/debts", nil)
	if number(listed["total"]) != 2 {
		t.Fatalf("debt list total = %v, want 2", listed["total"])
	}
	patched := c.mustOK("PATCH", "/api/v1/debts/"+loan, map[string]any{"status": "inactive", "note": "已结清停用"})
	if patched["status"] != "inactive" {
		t.Fatalf("patch status = %v", patched["status"])
	}
}

func cashIDForTest(t *testing.T, c *testClient) string {
	t.Helper()
	return c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "T2 付款卡", "opening_balance_minor": 500_000,
	})["id"].(string)
}

func TestV011_DebtPartialPaymentReversalAndIdempotency(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	loan := makeDebt(t, c, "loan", 300_000, "2026-11-01")
	makeDebtSchedule(t, c, loan, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 150_000, "interest_minor": 50_000, "fee_minor": 0,
	})

	c.headers = map[string]string{"Idempotency-Key": "debt-pay-1"}
	p1 := c.mustOK("POST", "/api/v1/debts/"+loan+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 80_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 60_000, "interest_minor": 20_000, "fee_minor": 0,
	})
	p1Again := c.mustOK("POST", "/api/v1/debts/"+loan+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 80_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 60_000, "interest_minor": 20_000, "fee_minor": 0,
	})
	c.headers = nil
	if p1["id"] != p1Again["id"] || p1Again["replayed"] != true {
		t.Fatalf("idempotent replay broken: %v / %v", p1, p1Again)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 420_000 {
		t.Errorf("replay double-booked cash: %d, want 420000", got)
	}
	detail := c.mustOK("GET", "/api/v1/debts/"+loan, nil)
	sched, _ := detail["schedule"].([]any)
	first := sched[0].(map[string]any)
	if first["status"] != "partial" || number(first["remaining_minor"]) != 120_000 {
		t.Fatalf("partial state wrong: %v", first)
	}

	// Settle the remainder explicitly.
	c.mustOK("POST", "/api/v1/debts/"+loan+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 120_000, "paid_on": "2026-12-11",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 90_000, "interest_minor": 30_000, "fee_minor": 0,
	})
	detail = c.mustOK("GET", "/api/v1/debts/"+loan, nil)
	sched, _ = detail["schedule"].([]any)
	first = sched[0].(map[string]any)
	if first["status"] != "paid" {
		t.Fatalf("period must be paid: %v", first)
	}
	if got := subjectMinor(t, database, "2030:"+loan); got != -150_000 {
		t.Errorf("loan liability after full payment = %d, want -150000", got)
	}

	// Reversal keeps both entries and restores balances/period state.
	rev := c.mustOK("POST", "/api/v1/debts/"+loan+"/payments/"+p1["id"].(string)+"/reverse", map[string]any{
		"reason": "重复录入纠正",
	})
	if rev["status"] != "reversed" {
		t.Fatalf("reverse response: %v", rev)
	}
	var reversedEntries int
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE biz_type='reversal'`).Scan(&reversedEntries)
	if reversedEntries != 1 {
		t.Errorf("reversal entry missing: %d", reversedEntries)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 380_000 {
		t.Errorf("cash after reversal = %d, want 380000", got)
	}
	detail = c.mustOK("GET", "/api/v1/debts/"+loan, nil)
	sched, _ = detail["schedule"].([]any)
	first = sched[0].(map[string]any)
	if first["status"] != "partial" || number(first["remaining_minor"]) != 80_000 {
		t.Errorf("period state after reversal = %v", first)
	}
	code, _ := c.do("POST", "/api/v1/debts/"+loan+"/payments/"+p1["id"].(string)+"/reverse", map[string]any{})
	if code == 200 || code == 201 {
		t.Fatal("double reversal must be rejected")
	}
}

func TestV011_DebtSettleEarlyKeepsHistory(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	loan := makeDebt(t, c, "loan", 300_000, "2026-11-01")
	makeDebtSchedule(t, c, loan,
		map[string]any{"installment_no": 1, "due_on": "2026-12-10", "principal_minor": 100_000, "interest_minor": 10_000, "fee_minor": 0},
		map[string]any{"installment_no": 2, "due_on": "2027-01-10", "principal_minor": 100_000, "interest_minor": 10_000, "fee_minor": 0},
		map[string]any{"installment_no": 3, "due_on": "2027-02-10", "principal_minor": 100_000, "interest_minor": 10_000, "fee_minor": 0},
	)
	c.mustOK("POST", "/api/v1/debts/"+loan+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 110_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 100_000, "interest_minor": 10_000, "fee_minor": 0,
	})
	out := c.mustOK("POST", "/api/v1/debts/"+loan+"/settle-early", map[string]any{"reason": "提前结清"})
	if number(out["cancelled_periods"]) != 2 {
		t.Fatalf("cancelled periods = %v, want 2", out["cancelled_periods"])
	}
	detail := c.mustOK("GET", "/api/v1/debts/"+loan, nil)
	sched, _ := detail["schedule"].([]any)
	if len(sched) != 3 {
		t.Fatalf("history must be preserved, got %d periods", len(sched))
	}
	states := []string{}
	for _, it := range sched {
		states = append(states, it.(map[string]any)["status"].(string))
	}
	if states[0] != "paid" || states[1] != "cancelled" || states[2] != "cancelled" {
		t.Errorf("period states = %v", states)
	}
	if got := subjectMinor(t, database, "2030:"+loan); got != -200_000 {
		t.Errorf("loan liability after early settlement = %d, want -200000", got)
	}
}

func TestV011_DebtPaymentValidation(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	loan := makeDebt(t, c, "loan", 200_000, "2026-11-01")
	makeDebtSchedule(t, c, loan, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 100_000, "interest_minor": 20_000, "fee_minor": 0,
	})
	base := map[string]any{
		"installment_no": 1, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
	}
	bad := []map[string]any{
		{"amount_minor": 50_000, "principal_minor": 40_000, "interest_minor": 5_000, "fee_minor": 0},    // sum mismatch
		{"amount_minor": 200_000, "principal_minor": 100_000, "interest_minor": 20_000, "fee_minor": 0}, // exceeds period
		{"amount_minor": 50_000, "principal_minor": 50_000, "interest_minor": 0, "fee_minor": 0, "channel": "bogus"},
		{"amount_minor": 50_000}, // no explicit split
	}
	for i, extra := range bad {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		code, out := c.do("POST", "/api/v1/debts/"+loan+"/payments", body)
		if code == 200 || code == 201 {
			t.Errorf("case %d must be rejected: %v", i, out)
		}
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 500_000 {
		t.Errorf("rejected payments must not move cash: %d", got)
	}
}

func makeInstallment(t *testing.T, c *testClient, principal, fee int64, periods int, firstDue string) string {
	t.Helper()
	return c.mustOK("POST", "/api/v1/installments", map[string]any{
		"name": "T3 分期", "category_id": c.firstCategory("expense"),
		"principal_minor": principal, "total_fee_minor": fee,
		"periods": periods, "first_due_on": firstDue,
	})["id"].(string)
}

func conservation(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	v, ok := out["conservation"].(map[string]any)
	if !ok {
		t.Fatalf("missing conservation block: %v", out)
	}
	return v
}

// T3: principal/fee conservation, explicit partial-payment split, and a
// partial payment that must NOT be auto-allocated.
func TestV011_T3_InstallmentConservationAndPartialPayment(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	inst := makeInstallment(t, c, 300_000, 9_000, 3, "2026-12-01")
	detail := c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	con := conservation(t, detail)
	if number(con["scheduled_principal_minor"]) != 300_000 || number(con["scheduled_fee_minor"]) != 9_000 ||
		number(con["total_due_minor"]) != 309_000 || con["principal_balanced"] != true || con["fee_balanced"] != true {
		t.Fatalf("conservation broken at creation: %v", con)
	}
	sched, _ := detail["schedule"].([]any)
	first := sched[0].(map[string]any)
	if number(first["principal_minor"]) != 100_000 || number(first["fee_minor"]) != 3_000 ||
		number(first["total_minor"]) != 103_000 {
		t.Fatalf("period 1 = %v", first)
	}

	// A partial payment WITHOUT an explicit split must be refused: none of the
	// 10 accounting policies (fee-first / principal-first / pro-rata) is confirmed.
	code, _ := c.do("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 51_500, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
	})
	if code == 200 || code == 201 {
		t.Fatal("partial payment without explicit split must be refused")
	}

	// Explicit split: 51500 = 50000 principal + 1500 fee.
	c.headers = map[string]string{"Idempotency-Key": "inst-pay-1"}
	p1 := c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 51_500, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 50_000, "fee_minor": 1_500,
	})
	p1Again := c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 51_500, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 50_000, "fee_minor": 1_500,
	})
	c.headers = nil
	if p1["id"] != p1Again["id"] || p1Again["replayed"] != true {
		t.Fatalf("idempotent replay broken: %v / %v", p1, p1Again)
	}
	detail = c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	sched, _ = detail["schedule"].([]any)
	first = sched[0].(map[string]any)
	if first["status"] != "partial" || number(first["remaining_minor"]) != 51_500 {
		t.Fatalf("period 1 partial state = %v", first)
	}
	con = conservation(t, detail)
	if number(con["paid_minor"]) != 51_500 || number(con["remaining_minor"]) != 257_500 {
		t.Fatalf("conservation after partial payment = %v", con)
	}
	if got := subjectMinor(t, database, "2020:"+inst); got != -250_000 {
		t.Errorf("installment liability = %d, want -250000", got)
	}
	if got := subjectMinor(t, database, "5110"); got != 1_500 {
		t.Errorf("period fee recognised = %d, want 1500", got)
	}

	// Settle the rest of period 1 explicitly.
	c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 51_500, "paid_on": "2026-12-02",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 50_000, "fee_minor": 1_500,
	})
	detail = c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	sched, _ = detail["schedule"].([]any)
	if sched[0].(map[string]any)["status"] != "paid" {
		t.Fatal("period 1 must be paid")
	}
	con = conservation(t, detail)
	if con["principal_balanced"] != true || con["fee_balanced"] != true {
		t.Errorf("conservation broken after settlement: %v", con)
	}

	// Daily projection repeated 20 times must remain idempotent (M4 gate).
	// The exact horizon depends on the business date, so we assert that only the
	// FIRST run may insert anything and the row count never grows afterwards.
	firstRun := c.mustOK("POST", "/api/v1/schedule/generate", map[string]any{})
	var afterFirst int
	_ = database.QueryRow(`SELECT COUNT(*) FROM schedule_due_items`).Scan(&afterFirst)
	for i := 1; i < 20; i++ {
		gen := c.mustOK("POST", "/api/v1/schedule/generate", map[string]any{})
		if number(gen["generated"]) != 0 {
			t.Errorf("run %d generated %v rows (must be idempotent)", i, gen["generated"])
		}
	}
	var afterAll int
	_ = database.QueryRow(`SELECT COUNT(*) FROM schedule_due_items`).Scan(&afterAll)
	if afterAll != afterFirst {
		t.Errorf("due items grew from %d to %d across 20 runs", afterFirst, afterAll)
	}
	_ = firstRun
}

func TestV011_InstallmentRefundAndExplicitScheduleAdjustment(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	inst := makeInstallment(t, c, 300_000, 9_000, 3, "2026-12-01")
	c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 103_000, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
	})
	// A refund must be linked to the original consumption and must NOT silently
	// delete paid periods. It only reports which future periods are affected.
	ref := c.mustOK("POST", "/api/v1/installments/"+inst+"/refunds", map[string]any{
		"amount_minor": 50_000, "refund_on": "2026-12-05",
		"destination_type": "cash", "destination_id": cash,
		"note": "部分退货",
	})
	if number(ref["refunded_minor"]) != 50_000 || ref["requires_explicit_adjustment"] != true {
		t.Fatalf("refund response = %v", ref)
	}
	affected, _ := ref["future_periods_affected"].([]any)
	if len(affected) != 2 {
		t.Fatalf("future periods affected = %d, want 2", len(affected))
	}
	detail := c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	sched, _ := detail["schedule"].([]any)
	if sched[0].(map[string]any)["status"] != "paid" {
		t.Fatal("paid period must be preserved after refund")
	}
	if number(conservation(t, detail)["refunded_minor"]) != 50_000 {
		t.Fatalf("refund not reflected in conservation: %v", detail["conservation"])
	}

	// Adjusting more than the refunded amount is refused (no unsupported relief).
	code, _ := c.do("POST", "/api/v1/installments/"+inst+"/adjust-schedule", map[string]any{
		"reason": "超额减免", "items": []any{map[string]any{
			"installment_no": 2, "action": "cancel",
		}},
	})
	if code == 200 || code == 201 {
		t.Fatal("adjustment exceeding total refunds must be refused")
	}

	// Explicit, audited reduction within the refunded amount.
	adj := c.mustOK("POST", "/api/v1/installments/"+inst+"/adjust-schedule", map[string]any{
		"reason": "按退款 500.00 调整第 2 期",
		"items": []any{map[string]any{
			"installment_no": 2, "action": "reduce",
			"principal_minor": 53_000, "fee_minor": 0,
		}},
	})
	if number(adj["reduced_minor"]) != 50_000 {
		t.Fatalf("reduced amount = %v, want 50000", adj["reduced_minor"])
	}
	detail = c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	sched, _ = detail["schedule"].([]any)
	if number(sched[1].(map[string]any)["total_minor"]) != 53_000 {
		t.Fatalf("period 2 after adjustment = %v", sched[1])
	}
	if number(conservation(t, detail)["adjustment_count"]) != 1 {
		t.Fatalf("adjustment not audited: %v", detail["conservation"])
	}
	// Paid history is untouched.
	if got := subjectMinor(t, database, "2020:"+inst); got != -200_000 {
		t.Errorf("liability after adjustment = %d, want -200000 (refund/adjustment must not silently rewrite posted entries)", got)
	}
}

func TestV011_InstallmentConvertFromCreditKeepsSinglePrincipal(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	expCat := c.firstCategory("expense")
	ci := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "转分期卡", "cycle_start_day": 1,
		"cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100_000, "occurred_on": "2026-11-12",
		"payment_kind": "credit", "credit_instrument_id": ci,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 100_000}},
	})
	if got := subjectMinor(t, database, "2010:"+ci); got != -100_000 {
		t.Fatalf("credit liability before conversion = %d", got)
	}
	out := c.mustOK("POST", "/api/v1/installments/convert-from-credit", map[string]any{
		"credit_instrument_id": ci, "name": "账单转分期",
		"principal_minor": 100_000, "total_fee_minor": 0,
		"periods": 2, "first_due_on": "2026-12-01",
	})
	instID := out["id"].(string)
	if got := subjectMinor(t, database, "2010:"+ci); got != 0 {
		t.Errorf("credit liability after conversion = %d, want 0", got)
	}
	if got := subjectMinor(t, database, "2020:"+instID); got != -100_000 {
		t.Errorf("installment liability = %d, want -100000", got)
	}
	sum := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	if number(sum["expense_minor"]) != 100_000 {
		t.Errorf("expense must be recognised exactly once, got %v", sum["expense_minor"])
	}
	detail := c.mustOK("GET", "/api/v1/installments/"+instID, nil)
	if con := conservation(t, detail); con["principal_balanced"] != true {
		t.Errorf("converted contract conservation = %v", con)
	}
	// Converting more than the outstanding credit balance is refused.
	code, _ := c.do("POST", "/api/v1/installments/convert-from-credit", map[string]any{
		"credit_instrument_id": ci, "name": "超额转分期",
		"principal_minor": 50_000, "total_fee_minor": 0,
		"periods": 2, "first_due_on": "2026-12-01",
	})
	if code == 200 || code == 201 {
		t.Fatal("conversion exceeding outstanding balance must be refused")
	}
}

func TestV011_InstallmentSettleEarlyKeepsPaidHistory(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := cashIDForTest(t, c)
	inst := makeInstallment(t, c, 300_000, 0, 3, "2026-12-01")
	c.mustOK("POST", "/api/v1/installments/"+inst+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 100_000, "paid_on": "2026-12-01",
		"channel": "cash", "channel_id": cash,
	})
	out := c.mustOK("POST", "/api/v1/installments/"+inst+"/settle-early", map[string]any{"reason": "提前结清"})
	if number(out["cancelled_periods"]) != 2 {
		t.Fatalf("cancelled periods = %v, want 2", out["cancelled_periods"])
	}
	detail := c.mustOK("GET", "/api/v1/installments/"+inst, nil)
	sched, _ := detail["schedule"].([]any)
	if len(sched) != 3 || sched[0].(map[string]any)["status"] != "paid" ||
		sched[1].(map[string]any)["status"] != "cancelled" || sched[2].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("periods after settlement = %v", sched)
	}
	if got := subjectMinor(t, database, "2020:"+inst); got != -200_000 {
		t.Errorf("liability after settlement = %d, want -200000", got)
	}
}
