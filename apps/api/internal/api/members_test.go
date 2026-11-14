package api

import "testing"

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
