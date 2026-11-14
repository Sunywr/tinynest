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
