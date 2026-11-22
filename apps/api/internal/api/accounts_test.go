package api

import "testing"

func TestAccountMaintenance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	member := c.mustOK("POST", "/api/v1/household/members", map[string]string{"name": "成员"})
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "旧账户", "kind": "cash", "opening_minor": 12345})
	path := "/api/v1/accounts/" + account["id"].(string)
	if code, _ := c.do("PATCH", path, map[string]string{"owner_member_id": "missing"}); code != 400 {
		t.Fatalf("unknown owner returned %d", code)
	}
	c.mustOK("PATCH", path, map[string]any{"name": "新账户", "owner_member_id": member["id"], "note": "备用"})
	got := c.mustOK("GET", path, nil)
	if got["name"] != "新账户" || got["owner_member_id"] != member["id"] || number(got["balance_minor"]) != 12345 || got["ledger_code"] != account["ledger_code"] {
		t.Fatalf("unexpected account: %v", got)
	}
	c.mustOK("PATCH", path, map[string]string{"owner_member_id": ""})
	if got := c.mustOK("GET", path, nil); got["owner_member_id"] != nil || got["note"] != "备用" {
		t.Fatalf("unexpected account after clearing owner: %v", got)
	}
}

func TestDisabledAccountRetainsBalance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "停用测试", "kind": "cash", "opening_minor": 1200})
	id := account["id"].(string)
	path := "/api/v1/accounts/" + id
	c.mustOK("PATCH", path, map[string]string{"status": "disabled"})
	got := c.mustOK("GET", path, nil)
	if got["status"] != "disabled" || number(got["balance_minor"]) != 1200 {
		t.Fatalf("unexpected account: %v", got)
	}
	if status, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"account_id": id, "category_id": c.firstCategory("expense"),
	}); status != 400 {
		t.Fatalf("disabled account accepted new payment: %d", status)
	}
	if code, _ := c.do("GET", path+"/journal", nil); code != 200 {
		t.Fatalf("journal read returned %d", code)
	}
	c.mustOK("PATCH", path, map[string]string{"status": "active"})
	if got := c.mustOK("GET", path, nil); got["status"] != "active" {
		t.Fatalf("reactivation failed: %v", got)
	}
}

func TestDeleteAccountRetainsLedger(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "待删除", "kind": "cash", "opening_minor": 5000})
	id := account["id"].(string)
	path := "/api/v1/accounts/" + id
	c.mustOK("DELETE", path, nil)
	c.mustOK("DELETE", path, nil) // idempotent soft delete
	got := c.mustOK("GET", path, nil)
	if got["status"] != "deleted" || number(got["balance_minor"]) != 5000 {
		t.Fatalf("deleted account lost its balance: %v", got)
	}
	for _, row := range v012Rows(t, c.mustOK("GET", "/api/v1/accounts", nil)) {
		if row["id"] == id {
			t.Fatalf("deleted account remained listed: %v", row)
		}
	}
	journal := c.mustOK("GET", path+"/journal", nil)["items"].([]any)
	if len(journal) != 1 || number(journal[0].(map[string]any)["change_minor"]) != 5000 {
		t.Fatalf("opening journal entry lost: %v", journal)
	}
}

// A credit account is a LIABILITY: its opening balance is booked against
// opening equity, so the derived balance is negative (= outstanding debt).
func TestCreditAccountOpeningIsLiability(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	credit := c.mustOK("POST", "/api/v1/accounts", map[string]any{"kind": "credit", "name": "信用卡", "opening_minor": 5000})
	got := c.mustOK("GET", "/api/v1/accounts/"+credit["id"].(string), nil)
	if number(got["balance_minor"]) != -5000 {
		t.Fatalf("credit balance = %v, want -5000 (liability)", got["balance_minor"])
	}
	// A cash receipt cannot be booked into a credit account.
	if status, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "income", "amount_minor": 100, "occurred_on": "2026-11-14",
		"account_id": credit["id"], "category_id": c.firstCategory("income"),
	}); status != 400 {
		t.Fatalf("income into credit account returned %d", status)
	}
}

func TestAccountJournalListsOpeningBalance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "账户流水", "kind": "cash", "opening_minor": 12345})
	body := c.mustOK("GET", "/api/v1/accounts/"+account["id"].(string)+"/journal", nil)
	items := body["items"].([]any)
	if len(items) != 1 || number(items[0].(map[string]any)["change_minor"]) != 12345 {
		t.Fatalf("unexpected journal: %v", items)
	}
}
