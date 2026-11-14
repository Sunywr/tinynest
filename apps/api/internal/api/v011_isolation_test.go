package api

import (
	"testing"
	"time"
)

func TestV011_T5_FinancialIsolation(t *testing.T) {
	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{Clock: func() time.Time { return now }, SetupToken: testSetupToken})
	type fixture struct {
		client                                   *remClient
		cash, card, statement, debt, installment string
		scale                                    int64
	}
	const root = "/api/v1"
	callOK := func(c *remClient, method, path string, body any, headers map[string]string) map[string]any {
		t.Helper()
		r := c.call(method, root+path, body, headers)
		if r.Status < 200 || r.Status >= 300 {
			t.Fatalf("%s %s: %d %v", method, path, r.Status, r.Body)
		}
		return r.Body
	}
	setup := func(suffix string, scale int64) fixture {
		t.Helper()
		h := bootstrapHousehold(t, e, suffix)
		f := fixture{client: h.client(t, e.ts.URL), scale: scale}
		post := func(path string, body any) map[string]any { return callOK(f.client, "POST", path, body, nil) }
		category := "expense_" + suffix
		if _, err := e.database.Exec(`INSERT INTO categories(id,household_id,kind,name,level) VALUES(?,?,'expense','测试支出',1)`, category, h.id); err != nil {
			t.Fatal(err)
		}
		f.cash = post("/accounts", map[string]any{"kind": "cash", "name": "测试账户", "opening_balance_minor": 100_000 * scale})["id"].(string)
		f.card = post("/credit-instruments", map[string]any{
			"kind": "credit_card", "name": "测试信用卡", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
		})["id"].(string)
		post("/transactions", map[string]any{
			"type": "expense", "amount_minor": 10_000 * scale, "occurred_on": "2026-11-13",
			"payment_kind": "credit", "credit_instrument_id": f.card,
			"splits": []map[string]any{{"category_id": category, "amount_minor": 10_000 * scale}},
		})
		f.statement = findRow(t, callOK(f.client, "GET", "/credit-statements", nil, nil), "instrument_id", f.card)["id"].(string)
		f.debt = post("/debts", map[string]any{"kind": "mortgage", "name": "测试贷款", "principal_minor": 20_000 * scale, "start_on": "2026-11-01", "due_day": 10, "cycle": "monthly"})["id"].(string)
		post("/debts/"+f.debt+"/schedule", map[string]any{"items": []map[string]any{
			{"installment_no": 1, "due_on": "2026-12-10", "principal_minor": 10_000 * scale, "interest_minor": 0, "fee_minor": 0},
		}})
		f.installment = post("/installments", map[string]any{
			"name": "测试分期", "category_id": category, "principal_minor": 20_000 * scale, "total_fee_minor": 0, "periods": 2, "first_due_on": "2026-12-10",
		})["id"].(string)
		return f
	}
	a, b := setup("t5a", 1), setup("t5b", 2)
	for _, pair := range [][2]fixture{{a, b}, {b, a}} {
		own, other := pair[0], pair[1]
		for _, path := range []string{"/accounts/" + other.cash + "/balance", "/credit-statements/" + other.statement, "/debts/" + other.debt, "/installments/" + other.installment} {
			if r := own.client.call("GET", root+path, nil, nil); r.Status != 404 {
				t.Errorf("foreign GET %s: got %d, want 404", path, r.Status)
			}
		}
		conversion := map[string]any{
			"credit_instrument_id": own.card, "statement_id": other.statement, "name": "跨家庭转分期",
			"principal_minor": 100, "periods": 2, "first_due_on": "2026-12-10",
		}
		if r := own.client.call("POST", root+"/installments/convert-from-credit", conversion, nil); r.Status != 400 {
			t.Errorf("foreign statement conversion: got %d, want 400", r.Status)
		}
		const paidOn = "2026-11-13"
		amount := 1000 * own.scale
		cardPayment := map[string]any{"from_account_id": own.cash, "amount_minor": amount, "paid_on": paidOn}
		debtPayment := map[string]any{"installment_no": 1, "channel": "cash", "channel_id": own.cash, "amount_minor": amount, "paid_on": paidOn, "principal_minor": amount, "interest_minor": 0, "fee_minor": 0}
		instPayment := map[string]any{"installment_no": 1, "channel": "cash", "channel_id": own.cash, "amount_minor": amount, "paid_on": paidOn, "principal_minor": amount, "fee_minor": 0}
		for _, payment := range []struct {
			prefix, ownID, otherID string
			body                   map[string]any
		}{
			{"/credit-statements/", own.statement, other.statement, cardPayment},
			{"/debts/", own.debt, other.debt, debtPayment},
			{"/installments/", own.installment, other.installment, instPayment},
		} {
			headers := map[string]string{"Idempotency-Key": "t5" + payment.prefix}
			path := payment.prefix + payment.ownID + "/payments"
			result := callOK(own.client, "POST", path, payment.body, headers)
			replay := callOK(own.client, "POST", path, payment.body, headers)
			if result["id"] == nil || replay["id"] != result["id"] || replay["replayed"] != true {
				t.Fatalf("payment %s replay = %v, original = %v", path, replay, result)
			}
			if r := own.client.call("POST", root+payment.prefix+payment.otherID+"/payments", payment.body, headers); r.Status != 400 && r.Status != 404 {
				t.Errorf("foreign payment %s: got %d, want 400 or 404", payment.prefix, r.Status)
			}
		}
	}
	for _, f := range []fixture{a, b} {
		balance := callOK(f.client, "GET", "/accounts/"+f.cash+"/balance", nil, nil)
		if got := number(balance["balance_minor"]); got != 97_000*f.scale {
			t.Errorf("cash balance for scale %d = %d", f.scale, got)
		}
		sum := callOK(f.client, "GET", "/dashboard/summary?from=2026-11-01&to=2026-12-31", nil, nil)
		if got := number(sum["expense_minor"]); got != 30_000*f.scale {
			t.Errorf("expense for scale %d = %d", f.scale, got)
		}
		callOK(f.client, "POST", "/schedule/generate", map[string]any{}, nil)
		due := callOK(f.client, "GET", "/schedule/due-items", nil, nil)
		items, _ := due["items"].([]any)
		if len(items) != 3 {
			t.Errorf("due items for scale %d = %v", f.scale, items)
		}
		for _, item := range items {
			if got := number(item.(map[string]any)["amount_minor"]); got != 9_000*f.scale {
				t.Errorf("due amount for scale %d = %d", f.scale, got)
			}
		}
	}
}
