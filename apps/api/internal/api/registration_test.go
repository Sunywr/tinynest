package api

import "testing"

func TestRegistrationRequiresApproval(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	credentials := map[string]string{"username": "new-member", "password": "valid-password-123"}
	c.mustOK("POST", "/api/v1/auth/register", credentials)
	invite := c.mustOK("POST", "/api/v1/household/invitations", map[string]any{"member_name": "新成员", "username": credentials["username"]})
	c.mustOK("POST", "/api/v1/auth/join", map[string]any{"username": credentials["username"], "password": credentials["password"], "code": invite["code"]})
	if status, _ := c.do("POST", "/api/v1/auth/session", credentials); status != 401 {
		t.Fatalf("unapproved login returned %d", status)
	}
	applications := c.mustOK("GET", "/api/v1/admin/join-applications", nil)["items"].([]any)
	path := "/api/v1/admin/join-applications/" + applications[0].(map[string]any)["id"].(string)
	c.mustOK("POST", path, map[string]bool{"approve": true})
	if status, _ := c.do("POST", path, map[string]bool{"approve": true}); status != 409 {
		t.Fatalf("replayed approval returned %d", status)
	}
	c.mustOK("POST", "/api/v1/auth/session", credentials)
	me := c.mustOK("GET", "/api/v1/auth/me", nil)
	if me["member_name"] != "新成员" || me["member_id"] == "" {
		t.Fatalf("missing member identity: %v", me)
	}
	var bound string
	if err := database.QueryRow("SELECT member_id FROM household_user_members WHERE user_id=?", me["user_id"]).Scan(&bound); err != nil || bound != me["member_id"] {
		t.Fatalf("invalid binding: %s %v", bound, err)
	}
}
func TestRevokedInvitationCannotBeUsed(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	member := c.mustOK("POST", "/api/v1/household/members", map[string]string{"name": "邀请对象"})
	invite := c.mustOK("POST", "/api/v1/household/invitations", map[string]any{"member_id": member["id"]})
	path := "/api/v1/admin/invitations"
	items := c.mustOK("GET", path, nil)["items"].([]any)
	id := items[0].(map[string]any)["id"].(string)
	c.mustOK("DELETE", path+"/"+id, nil)
	credentials := map[string]string{"username": "revoked-test", "password": "valid-password-123"}
	c.mustOK("POST", "/api/v1/auth/register", credentials)
	credentials["code"] = invite["code"].(string)
	if code, _ := c.do("POST", "/api/v1/auth/join", credentials); code != 400 {
		t.Fatalf("revoked invite returned %d", code)
	}
}
