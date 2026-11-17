package api

import "testing"

func TestCurrentHouseholdMember(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	path := "/api/v1/household/members/current"
	first := client.mustOK("POST", path, nil)
	second := client.mustOK("POST", path, nil)
	me := client.mustOK("GET", "/api/v1/auth/me", nil)
	if first["id"] == "" || first["id"] == nil || first["id"] != second["id"] ||
		first["id"] != me["member_id"] || first["name"] != me["username"] {
		t.Fatalf("current member must be bound exactly once: %v, %v, %v", first, second, me["member_id"])
	}
	items := client.mustOK("GET", "/api/v1/household/members", nil)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one member profile, got %d", len(items))
	}
}

func TestHouseholdMemberDirectory(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	path := "/api/v1/household/members"
	client.mustOK("POST", path, map[string]string{"name": "  家庭成员  "})
	items := client.mustOK("GET", path, nil)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "家庭成员" {
		t.Fatalf("unexpected member directory: %v", items)
	}
	code, _ := client.do("POST", path, map[string]string{"name": " "})
	if code != 400 {
		t.Fatalf("blank name returned %d", code)
	}
}

func TestDeleteUnusedHouseholdMember(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	const base = "/api/v1/household/members"
	unused := c.mustOK("POST", base, map[string]string{"name": "unused"})["id"].(string)
	c.mustOK("DELETE", base+"/"+unused, nil)
	c.mustOK("DELETE", base+"/"+unused, nil)
	bound := c.mustOK("POST", base+"/current", nil)["id"].(string)
	if status, _ := c.do("DELETE", base+"/"+bound, nil); status != 409 {
		t.Fatalf("bound member returned %d", status)
	}
	if items := c.mustOK("GET", base, nil)["items"].([]any); len(items) != 1 {
		t.Fatalf("unexpected directory: %v", items)
	}
	invited := c.mustOK("POST", base, map[string]string{"name": "invited"})["id"].(string)
	c.mustOK("POST", "/api/v1/household/invitations", map[string]string{"member_id": invited})
	if status, _ := c.do("DELETE", base+"/"+invited, nil); status != 409 {
		t.Fatalf("invited member returned %d", status)
	}
	owner := c.mustOK("POST", base, map[string]string{"name": "owner"})["id"].(string)
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"name": "测试账户", "kind": "cash", "owner_member_id": owner,
	})["id"].(string)
	if status, _ := c.do("DELETE", base+"/"+owner, nil); status != 409 {
		t.Fatalf("account owner returned %d", status)
	}
	if status, _ := c.do("PATCH", "/api/v1/accounts/"+account, map[string]string{"owner_member_id": unused}); status != 400 {
		t.Fatalf("deleted member accepted as account owner: %d", status)
	}
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"payment_kind": "cash", "account_id": account, "member_id": owner,
		"splits": []map[string]any{{"category_id": c.firstCategory("expense"), "amount_minor": 100}},
	})
	c.mustOK("PATCH", "/api/v1/accounts/"+account, map[string]string{"owner_member_id": ""})
	if status, _ := c.do("DELETE", base+"/"+owner, nil); status != 409 {
		t.Fatalf("historical member returned %d", status)
	}
}
