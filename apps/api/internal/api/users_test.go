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
	user := admin.mustOK("POST", base, map[string]string{"username": "member-test", "password": "member-test-password"})
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
	admin.mustOK("POST", base+"/"+user["id"].(string)+"/disable", nil)
	if code, _ := member.do("GET", "/api/v1/accounts", nil); code != 401 {
		t.Fatalf("disabled member session returned %d", code)
	}
}
