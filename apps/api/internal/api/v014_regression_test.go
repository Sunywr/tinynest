package api

import (
	"testing"
)

// RC-M3: insurance premium end-to-end fixed-number regression.
//
// The acceptance checklist requires insurance to be checked alongside cash,
// credit, mortgage, installments, investment and homepage statistics. The
// kernel already implements the rule (fee -> 5120, never a loan principal),
// but there was no end-to-end fixed-number assertion tying the posting to the
// dashboard and net-worth views. This test adds that evidence through real
// HTTP + real SQLite; it does not add or change any accounting behaviour.
func TestV014_InsurancePremiumReconciles(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "RC 保险付款卡", "opening_balance_minor": 1_000_000,
	})["id"].(string)

	// Insurance has no loan principal by rule.
	insurance := makeDebt(t, c, "insurance", 0, "2026-11-01")
	makeDebtSchedule(t, c, insurance, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 0, "interest_minor": 0, "fee_minor": 12_000,
	})

	pay := c.mustOK("POST", "/api/v1/debts/"+insurance+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 12_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 0, "interest_minor": 0, "fee_minor": 12_000,
	})
	if pay["status"] != "confirmed" {
		t.Fatalf("insurance payment not confirmed: %v", pay)
	}

	// 1) Premium is an insurance expense (5120), never interest (5100) or
	//    generic fee (5110), and never a loan principal liability (2030).
	if got := subjectMinor(t, database, "5120"); got != 12_000 {
		t.Errorf("insurance expense = %d, want 12000", got)
	}
	if got := subjectMinor(t, database, "5100"); got != 0 {
		t.Errorf("interest expense must stay 0 for insurance, got %d", got)
	}
	if got := subjectMinor(t, database, "5110"); got != 0 {
		t.Errorf("generic fee must stay 0 for insurance, got %d", got)
	}
	if got := subjectMinor(t, database, "2030:"+insurance); got != 0 {
		t.Errorf("insurance must not create a loan principal liability, got %d", got)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 988_000 {
		t.Errorf("cash after premium = %d, want 988000", got)
	}

	// 2) Homepage three-basis reporting must include the premium as expense,
	//    count the cash outflow once, and keep net worth reconciled.
	sum := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-12-01&to=2026-12-31", nil)
	if got := number(sum["expense_minor"]); got != 12_000 {
		t.Errorf("December expense must be 12000 (premium only), got %d", got)
	}
	if got := number(sum["cashflow_minor"]); got != -12_000 {
		t.Errorf("December cashflow must be -12000, got %d", got)
	}
	nw := c.mustOK("GET", "/api/v1/dashboard/net-worth?from=2026-12-01&to=2026-12-31", nil)
	if got, want := number(nw["net_worth_minor"]), number(sum["net_worth_minor"]); got != want {
		t.Errorf("net worth %d != dashboard summary %d", got, want)
	}
	if got := number(nw["net_worth_minor"]); got != 988_000 {
		t.Errorf("net worth after premium = %d, want 988000", got)
	}

	// 3) Reversal of a booked insurance payment must remove the premium but
	//    keep the contract history (immutable journal + reversal entry).
	payID := pay["id"].(string)
	c.mustOK("POST", "/api/v1/debts/"+insurance+"/payments/"+payID+"/reverse", map[string]any{
		"reversed_on": "2026-12-11",
	})
	if got := subjectMinor(t, database, "5120"); got != 0 {
		t.Errorf("premium must be reversed to 0, got %d", got)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 1_000_000 {
		t.Errorf("cash must return to 1000000 after reversal, got %d", got)
	}
}

// Insurance must refuse any attempt to smuggle a principal component into a
// schedule item or a payment split; no automatic allocation is allowed.
func TestV014_InsuranceRejectsPrincipalInEveryPath(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	code, _ := c.do("POST", "/api/v1/debts", map[string]any{
		"kind": "insurance", "name": "带本金保险", "principal_minor": 50_000, "start_on": "2026-11-01",
	})
	if code == 200 || code == 201 {
		t.Error("insurance contract with principal must be rejected")
	}

	ins := makeDebt(t, c, "insurance", 0, "2026-11-01")
	code, _ = c.do("POST", "/api/v1/debts/"+ins+"/schedule", map[string]any{
		"items": []any{map[string]any{
			"installment_no": 1, "due_on": "2026-12-10",
			"principal_minor": 10_000, "interest_minor": 0, "fee_minor": 0,
		}},
	})
	if code == 200 || code == 201 {
		t.Error("insurance schedule with principal must be rejected")
	}

	makeDebtSchedule(t, c, ins, map[string]any{
		"installment_no": 1, "due_on": "2026-12-10",
		"principal_minor": 0, "interest_minor": 0, "fee_minor": 12_000,
	})
	cash := cashIDForTest(t, c)
	code, _ = c.do("POST", "/api/v1/debts/"+ins+"/payments", map[string]any{
		"installment_no": 1, "amount_minor": 12_000, "paid_on": "2026-12-10",
		"channel": "cash", "channel_id": cash,
		"principal_minor": 1, "interest_minor": 0, "fee_minor": 11_999,
	})
	if code == 200 || code == 201 {
		t.Error("insurance payment with a principal split must be rejected")
	}
}
