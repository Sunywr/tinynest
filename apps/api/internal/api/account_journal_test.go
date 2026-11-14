package api

import "testing"

func TestAccountJournalListsOpeningBalance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "账户流水", "kind": "cash", "opening_balance_minor": 12345})
	body := c.mustOK("GET", "/api/v1/accounts/"+account["id"].(string)+"/journal", nil)
	items := body["items"].([]any)
	if len(items) != 1 || number(items[0].(map[string]any)["change_minor"]) != 12345 {
		t.Fatalf("unexpected journal: %v", items)
	}
}
