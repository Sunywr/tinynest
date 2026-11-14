package api

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestMemberAccountLifecycle(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	base := "/api/v1/admin/users"
	credentials := map[string]string{"username": "member-test", "password": "member-test-password"}
	profile := admin.mustOK("POST", "/api/v1/household/members", map[string]string{"name": "成员"})
	admin.mustOK("POST", "/api/v1/auth/register", credentials)
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
	admin.mustOK("POST", base+"/"+member.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)+"/disable", nil)
	if code, _ := member.do("GET", "/api/v1/accounts", nil); code != 401 {
		t.Fatalf("disabled member session returned %d", code)
	}
}
