package api

import "testing"

func TestAccountMaintenance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	member := c.mustOK("POST", "/api/v1/household/members", map[string]string{"name": "成员"})
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "旧账户", "kind": "cash", "opening_balance_minor": 12345})
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
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "停用测试", "kind": "cash", "opening_balance_minor": 1200})
	id := account["id"].(string)
	path := "/api/v1/accounts/" + id
	c.mustOK("PATCH", path, map[string]string{"status": "disabled"})
	got := c.mustOK("GET", path, nil)
	if got["status"] != "disabled" || number(got["balance_minor"]) != 1200 {
		t.Fatalf("unexpected account: %v", got)
	}
	if status, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"payment_kind": "cash", "account_id": id,
		"splits": []map[string]any{{"category_id": c.firstCategory("expense"), "amount_minor": 100}},
	}); status != 400 {
		t.Fatalf("disabled account accepted new payment: %d", status)
	}

	dashboard := c.mustOK("GET", "/api/v1/dashboard/summary", nil)
	if len(dashboard["accounts"].([]any)) != 1 {
		t.Fatalf("disabled account missing from dashboard: %v", dashboard)
	}
	c.mustOK("GET", path+"/journal", nil)
	c.mustOK("PATCH", path, map[string]string{"status": "active"})
	if got := c.mustOK("GET", path, nil); got["status"] != "active" {
		t.Fatalf("reactivation failed: %v", got)
	}
}

func TestDeleteAccountRetainsLedger(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"name": "待删除账户", "kind": "cash", "opening_balance_minor": 12345,
	})
	path := "/api/v1/accounts/" + account["id"].(string)
	c.mustOK("DELETE", path, nil)
	c.mustOK("DELETE", path, nil)

	list := c.mustOK("GET", "/api/v1/accounts", nil)
	if number(list["total"]) != 0 {
		t.Fatalf("deleted account visible in list: %v", list)
	}
	dashboard := c.mustOK("GET", "/api/v1/dashboard/summary", nil)
	if len(dashboard["accounts"].([]any)) != 0 || number(dashboard["net_worth_minor"]) != 12345 {
		t.Fatalf("deletion changed dashboard accounting or left the account listed: %v", dashboard)
	}
	if status, _ := c.do("DELETE", "/api/v1/accounts/missing", nil); status != 404 {
		t.Fatalf("missing account deletion returned %d", status)
	}
	if status, _ := c.do("PATCH", path, map[string]string{"status": "active"}); status != 404 {
		t.Fatalf("deleted account could be edited: %d", status)
	}
	if got := c.mustOK("GET", path+"/balance", nil); number(got["balance_minor"]) != 12345 {
		t.Fatalf("deleted account lost its ledger balance: %v", got)
	}
	c.mustOK("GET", path+"/journal", nil)
}
