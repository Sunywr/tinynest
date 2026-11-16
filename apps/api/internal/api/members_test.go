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
