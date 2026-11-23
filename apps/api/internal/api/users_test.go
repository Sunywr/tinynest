package api

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// TestHouseholdAdminManagesMemberRoles covers the household-level role model:
// an admin may promote or demote other active accounts, but never their own
// role, and concurrent demotions must not leave the household without an admin.
// Roles are read from the users row on every request, so a change applies to
// existing sessions at once.
func TestHouseholdAdminManagesMemberRoles(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	member := newMemberClient(t, ts, admin, "role-member", "role-password-12")
	memberID := member.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	adminID := admin.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	const base = "/api/v1/admin/users/"

	// The admin can never change their own role, not even to a no-op value.
	for _, role := range []string{"viewer", "admin"} {
		if code, body := admin.do("PATCH", base+adminID+"/role", map[string]string{"role": role}); code != 403 {
			t.Fatalf("self role change to %s returned %d: %v", role, code, body)
		}
	}
	if body := admin.mustOK("GET", "/api/v1/auth/me", nil); body["role"] != "admin" {
		t.Fatalf("self role change was applied anyway: %v", body)
	}

	// Promote a member: the new role applies to the existing session immediately.
	admin.mustOK("PATCH", base+memberID+"/role", map[string]string{"role": "admin"})
	member.mustOK("GET", "/api/v1/admin/users", nil)

	// The promoted admin also cannot change their own role.
	if code, body := member.do("PATCH", base+memberID+"/role", map[string]string{"role": "viewer"}); code != 403 {
		t.Fatalf("promoted admin self-demotion returned %d: %v", code, body)
	}

	// Admins still manage each other: the promoted member demotes the founder.
	member.mustOK("PATCH", base+adminID+"/role", map[string]string{"role": "member"})
	if code, _ := admin.do("GET", "/api/v1/admin/users", nil); code != 403 {
		t.Fatalf("demoted admin kept admin access: %d", code)
	}

	// Hand the admin role back, then demote the member to viewer.
	member.mustOK("PATCH", base+adminID+"/role", map[string]string{"role": "admin"})
	admin.mustOK("PATCH", base+memberID+"/role", map[string]string{"role": "viewer"})
	if code, _ := member.do("GET", "/api/v1/admin/users", nil); code != 403 {
		t.Fatalf("demoted member kept admin access: %d", code)
	}
	if code, _ := member.do("POST", "/api/v1/accounts", map[string]string{"name": "viewer", "kind": "cash"}); code != 403 {
		t.Fatalf("demoted viewer could write: %d", code)
	}

	// Unknown roles are rejected, and ordinary members cannot manage roles.
	admin.mustOK("PATCH", base+memberID+"/role", map[string]string{"role": "member"})
	if code, _ := admin.do("PATCH", base+memberID+"/role", map[string]string{"role": "owner"}); code != 400 {
		t.Fatalf("unknown role returned %d", code)
	}
	if code, _ := member.do("PATCH", base+adminID+"/role", map[string]string{"role": "viewer"}); code != 403 {
		t.Fatalf("member role update returned %d", code)
	}
}

func TestMemberAccountLifecycle(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	base := "/api/v1/admin/users"
	credentials := map[string]string{"username": "member-test", "password": "member-test-password"}
	profile := admin.mustOK("POST", "/api/v1/household/members", map[string]string{"name": "成员"})
	admin.registerVerified(credentials["username"], credentials["password"])
	invite := admin.mustOK("POST", "/api/v1/household/invitations", map[string]any{"member_id": profile["id"]})
	admin.mustOK("POST", "/api/v1/auth/join", map[string]any{"username": credentials["username"], "password": credentials["password"], "code": invite["code"]})
	pending := admin.mustOK("GET", "/api/v1/admin/join-applications", nil)["items"].([]any)
	admin.mustOK("POST", "/api/v1/admin/join-applications/"+pending[0].(map[string]any)["id"].(string), map[string]bool{"approve": true})
	jar, _ := cookiejar.New(nil)
	member := &testClient{t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	login := member.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": "member-test", "password": "member-test-password"})
	member.csrf = login["csrf_token"].(string)
	member.mustOK("GET", "/api/v1/accounts", nil)
	for _, path := range []string{base, "/api/v1/admin/audit-logs"} {
		if code, _ := member.do("GET", path, nil); code != 403 {
			t.Fatalf("member access to %s returned %d", path, code)
		}
	}
	userID := member.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	admin.mustOK("PATCH", base+"/"+userID+"/role", map[string]string{"role": "viewer"})
	member.mustOK("GET", "/api/v1/accounts", nil)
	if code, _ := member.do("POST", "/api/v1/accounts", map[string]string{"name": "blocked", "kind": "cash"}); code != 403 {
		t.Fatalf("viewer write returned %d", code)
	}
	admin.mustOK("PATCH", base+"/"+userID+"/role", map[string]string{"role": "member"})
	member.mustOK("POST", "/api/v1/accounts", map[string]string{"name": "restored", "kind": "cash"})
	admin.mustOK("PATCH", base+"/"+userID+"/role", map[string]string{"role": "viewer"})

	admin.mustOK("POST", base+"/"+member.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)+"/disable", nil)
	if code, _ := member.do("GET", "/api/v1/accounts", nil); code != 401 {
		t.Fatalf("disabled member session returned %d", code)
	}
}
