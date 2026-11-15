package api

import "testing"

func TestTransactionPagination(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Paging account", "opening_balance_minor": 1000,
	})["id"]
	category := c.firstCategory("expense")
	for range 201 {
		c.mustOK("POST", "/api/v1/transactions", map[string]any{
			"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-12",
			"payment_kind": "cash", "account_id": account,
			"splits": []any{map[string]any{"category_id": category, "amount_minor": 100}},
		})
	}
	const filter = "/api/v1/transactions?from=2026-11-12&to=2026-11-12&type=expense"
	first := c.mustOK("GET", filter, nil)
	cursor, ok := first["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("missing continuation cursor: %v", first)
	}
	second := c.mustOK("GET", filter+"&cursor="+cursor, nil)
	secondItems := second["items"].([]any)
	firstItems := first["items"].([]any)
	if len(firstItems) != 200 || len(secondItems) != 1 || second["next_cursor"] != nil ||
		firstItems[199].(map[string]any)["id"] != cursor {
		t.Fatalf("unexpected page sizes or cursors: sizes %d/%d, final cursor %v", len(firstItems), len(secondItems), second["next_cursor"])
	}
	for _, item := range firstItems {
		if item.(map[string]any)["id"] == secondItems[0].(map[string]any)["id"] {
			t.Fatal("second page repeats a transaction from the first")
		}
	}
	for _, query := range []string{"&cursor=missing", "&limit=201"} {
		if status, _ := c.do("GET", filter+query, nil); status != 400 {
			t.Errorf("%s: status = %d, want 400", query, status)
		}
	}
}
