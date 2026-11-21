package api

import "testing"

func TestBatchStatementPaymentAtomicReplay(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "cash", "name": "Cash", "opening_balance_minor": 1000})["id"].(string)
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{"kind": "credit_card", "name": "Card", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1})["id"]
	cat := c.firstCategory("expense")
	for _, day := range []string{"2026-09-15", "2026-10-15"} {
		c.mustOK("POST", "/api/v1/transactions", map[string]any{"type": "expense", "amount_minor": 100, "occurred_on": day, "payment_kind": "credit", "credit_instrument_id": card, "splits": []any{map[string]any{"category_id": cat, "amount_minor": 100}}})
	}
	rows := c.mustOK("GET", "/api/v1/credit-statements?unpaid=true", nil)["items"].([]any)
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	first := rows[0].(map[string]any)["id"].(string)
	second := rows[1].(map[string]any)["id"].(string)
	c.mustOK("POST", "/api/v1/credit-statements/"+first+"/payments", map[string]any{"amount_minor": 20, "paid_on": "2026-11-12", "from_account_id": account})
	items := []map[string]any{{"id": first, "amount_minor": 80}, {"id": second, "amount_minor": 90}}
	body := map[string]any{"from": "2026-10-10", "to": "2026-11-10", "paid_on": "2026-11-12", "from_account_id": account, "items": items}
	c.headers = map[string]string{"Idempotency-Key": "batch-test"}
	const path = "/api/v1/credit-statement-batch-payments"
	if status, _ := c.do("POST", path, body); status != 400 {
		t.Fatal(status)
	}
	balance := func() int64 {
		return number(c.mustOK("GET", "/api/v1/accounts/"+account+"/balance", nil)["balance_minor"])
	}
	if balance() != 980 {
		t.Fatal("partial batch was posted")
	}
	items[1]["amount_minor"] = 100
	c.mustOK("POST", path, body)
	c.mustOK("POST", path, body)
	if balance() != 800 {
		t.Fatal("incorrect balance after replay")
	}
}
