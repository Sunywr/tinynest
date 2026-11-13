package api

import (
	"testing"
	"time"

	"tinynest/api/internal/credit"
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
