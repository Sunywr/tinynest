package api

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"
)

// User profile self-service and admin user management. Names live on the bound
// household_members row; avatar/birthday/payday/email live on users. Usernames
// are login identities and can never be renamed, by anyone.

func newMemberClient(t *testing.T, ts *httptest.Server, admin *testClient, username, password string) *testClient {
	t.Helper()
	profile := admin.mustOK("POST", "/api/v1/household/members", map[string]string{"name": username})
	admin.registerVerified(username, password)
	invite := admin.mustOK("POST", "/api/v1/household/invitations", map[string]any{"member_id": profile["id"]})
	admin.mustOK("POST", "/api/v1/auth/join", map[string]any{"username": username, "password": password, "code": invite["code"]})
	for _, it := range admin.mustOK("GET", "/api/v1/admin/join-applications", nil)["items"].([]any) {
		m := it.(map[string]any)
		if m["username"] == username {
			admin.mustOK("POST", "/api/v1/admin/join-applications/"+m["id"].(string), map[string]bool{"approve": true})
		}
	}
	jar, _ := cookiejar.New(nil)
	c := &testClient{mailAPI: admin.mailAPI, t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	login := c.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": username, "password": password})
	c.csrf = login["csrf_token"].(string)
	return c
}

func TestUserSelfServiceProfileAndPassword(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	registrationAttempts = newLoginLimiter(LoginLimiterConfig{MaxFailures: 8}, time.Now)
	member := newMemberClient(t, ts, admin, "member-self", "member-password-12")

	member.mustOK("PATCH", "/api/v1/auth/me/profile", map[string]any{
		"name": "小新", "avatar": "🐱", "birthday": "1995-05-20", "payday": 25,
	})
	me := member.mustOK("GET", "/api/v1/auth/me", nil)
	if me["member_name"] != "小新" || me["avatar"] != "🐱" || me["birthday"] != "1995-05-20" ||
		number(me["payday"]) != 25 || me["email"] != "member-self@example.com" {
		t.Fatalf("profile not persisted: %v", me)
	}
	// 姓名 flows to the bound member profile used by transaction attribution.
	members := v012Rows(t, admin.mustOK("GET", "/api/v1/household/members", nil))
	found := false
	for _, m := range members {
		if m["name"] == "小新" {
			found = true
		}
	}
	if !found {
		t.Fatalf("member profile name not updated: %v", members)
	}

	// Username is a login identity and cannot be renamed, even by its owner.
	if code, body := member.do("PATCH", "/api/v1/auth/me/profile", map[string]any{"username": "hacked"}); code != 400 {
		t.Fatalf("username rename returned %d: %v", code, body)
	}
	for _, bad := range []map[string]any{
		{"birthday": "1995-13-40"},
		{"birthday": "2999-01-01"},
		{"payday": 32},
		{"payday": 0},
		{"email": "not-an-email"},
		{"avatar": "abcdefghijkl"},
		{"name": ""},
	} {
		if code, body := member.do("PATCH", "/api/v1/auth/me/profile", bad); code != 400 {
			t.Fatalf("invalid profile %v returned %d: %v", bad, code, body)
		}
	}
	// Clearing optional fields is explicit.
	if status, _ := member.do("PATCH", "/api/v1/auth/me/profile", map[string]any{"email": nil}); status != 400 {
		t.Fatal("email cleared without verification")
	}
	member.mustOK("PATCH", "/api/v1/auth/me/profile", map[string]any{"payday": nil})
	me = member.mustOK("GET", "/api/v1/auth/me", nil)
	if _, ok := me["email"]; !ok {
		t.Fatalf("email not cleared: %v", me)
	}

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 64, 65)), nil); err != nil {
		t.Fatal(err)
	}
	invalidImage := avatarJPEGPrefix + base64.StdEncoding.EncodeToString(buffer.Bytes())
	if status, _ := member.do("PATCH", "/api/v1/auth/me/profile", map[string]any{"avatar": invalidImage}); status != 400 {
		t.Fatalf("wrong image dimensions returned %d", status)
	}
	buffer.Reset()
	if err := jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 64, 64)), nil); err != nil {
		t.Fatal(err)
	}
	avatar := avatarJPEGPrefix + base64.StdEncoding.EncodeToString(buffer.Bytes())
	member.mustOK("PATCH", "/api/v1/auth/me/profile", map[string]any{"avatar": avatar})
	if got := member.mustOK("GET", "/api/v1/auth/me", nil)["avatar"]; got != avatar {
		t.Fatal("avatar image not persisted")
	}

	// Password change: correct current password required, session survives.
	if code, _ := member.do("POST", "/api/v1/auth/me/password", map[string]any{"current_password": "wrong", "new_password": "brand-new-pass-99"}); code != 400 {
		t.Fatalf("wrong current password returned %d", code)
	}
	if code, _ := member.do("POST", "/api/v1/auth/me/password", map[string]any{"current_password": "member-password-12", "new_password": "short"}); code != 400 {
		t.Fatalf("short new password returned %d", code)
	}
	member.changePasswordVerified("member-password-12", "brand-new-pass-99")
	if status, _ := member.do("GET", "/api/v1/auth/me", nil); status != 401 {
		t.Fatal("session survived password change")
	}

	jar, _ := cookiejar.New(nil)
	relogin := &testClient{t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	relogin.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": "member-self", "password": "brand-new-pass-99"})
	if code, _ := relogin.do("POST", "/api/v1/auth/session", map[string]string{"username": "member-self", "password": "member-password-12"}); code != 401 {
		t.Fatalf("old password still works: %d", code)
	}
}

func TestViewerSelfServiceAllowed(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	registrationAttempts = newLoginLimiter(LoginLimiterConfig{MaxFailures: 8}, time.Now)
	viewer := newMemberClient(t, ts, admin, "viewer-self", "viewer-password-12")
	userID := viewer.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	admin.mustOK("PATCH", "/api/v1/admin/users/"+userID+"/role", map[string]string{"role": "viewer"})

	viewer.mustOK("PATCH", "/api/v1/auth/me/profile", map[string]any{"name": "只读君", "birthday": "1990-01-02"})
	viewer.changePasswordVerified("viewer-password-12", "viewer-new-pass-99")
	login := viewer.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": "viewer-self", "password": "viewer-new-pass-99"})
	viewer.csrf = login["csrf_token"].(string)
	if me := viewer.mustOK("GET", "/api/v1/auth/me", nil); me["member_name"] != "只读君" {
		t.Fatalf("viewer profile not updated: %v", me)
	}
	// Ledger writes stay forbidden for viewer accounts.
	if code, _ := viewer.do("POST", "/api/v1/accounts", map[string]string{"name": "blocked", "kind": "cash"}); code != 403 {
		t.Fatalf("viewer ledger write returned %d", code)
	}
}

func TestAdminManagesUserProfilesAndDeletion(t *testing.T) {
	ts, admin, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	registrationAttempts = newLoginLimiter(LoginLimiterConfig{MaxFailures: 8}, time.Now)
	m1 := newMemberClient(t, ts, admin, "managed-one", "managed-password-1")
	m2 := newMemberClient(t, ts, admin, "managed-two", "managed-password-2")
	m2id := m2.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)

	foreign := bootstrapHousehold(t, &remEnv{database: database}, "profile")
	outsider := foreign.client(t, ts.URL)
	for _, attempt := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/profile", map[string]any{"name": "其他家庭"}},
		{"POST", "/password", map[string]any{"new_password": "foreign-pass-0000"}},
		{"DELETE", "", nil},
	} {
		result := outsider.call(attempt.method, "/api/v1/admin/users/"+m2id+attempt.path, attempt.body, nil)
		if result.Status != 404 && !(attempt.path == "/password" && result.Status == 403) {
			t.Fatalf("cross-household %s returned %d: %v", attempt.method, result.Status, result.Body)
		}
	}

	// The managed member books a transaction before deletion; history must survive.
	m2.mustOK("POST", "/api/v1/accounts", map[string]any{"name": "成员账户", "kind": "cash", "opening_balance_minor": 5000})
	accounts := v012Rows(t, m2.mustOK("GET", "/api/v1/accounts", nil))
	m2.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 1200, "occurred_on": "2026-11-10",
		"payment_kind": "cash", "account_id": accounts[0]["id"],
		"splits": []any{map[string]any{"category_id": m2.firstCategory("expense"), "amount_minor": 1200}},
		"note":   "删除前记录",
	})

	admin.mustOK("PATCH", "/api/v1/admin/users/"+m2id+"/profile", map[string]any{
		"name": "被管理者", "avatar": "🐶", "birthday": "1988-08-08", "payday": 10,
	})
	if me := m2.mustOK("GET", "/api/v1/auth/me", nil); me["member_name"] != "被管理者" || me["avatar"] != "🐶" {
		t.Fatalf("admin profile update not visible: %v", me)
	}
	if code, _ := admin.do("PATCH", "/api/v1/admin/users/"+m2id+"/profile", map[string]any{"username": "renamed"}); code != 400 {
		t.Fatalf("admin username rename returned %d", code)
	}

	// Admin password reset: old sessions revoked, new password works.
	if status, _ := admin.do("POST", "/api/v1/admin/users/"+m2id+"/password", map[string]any{"new_password": "reset-pass-0000"}); status != 403 {
		t.Fatal("admin bypassed verification")
	}
	m2.changePasswordVerified("managed-password-2", "reset-pass-0000")
	if code, _ := m2.do("GET", "/api/v1/auth/me", nil); code != 401 {
		t.Fatalf("session not revoked after reset: %d", code)
	}
	jar, _ := cookiejar.New(nil)
	relogin := &testClient{t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	relogin.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": "managed-two", "password": "reset-pass-0000"})
	m2 = relogin
	m2.csrf = m2.mustOK("GET", "/api/v1/auth/me", nil)["csrf_token"].(string)

	// Admin cannot be deleted; unknown users 404.
	adminID := admin.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	if code, _ := admin.do("DELETE", "/api/v1/admin/users/"+adminID, nil); code != 404 {
		t.Fatalf("admin self-delete returned %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/admin/users/user_nope", nil); code != 404 {
		t.Fatalf("unknown user delete returned %d", code)
	}
	// Members cannot manage other users.
	if code, _ := m1.do("PATCH", "/api/v1/admin/users/"+m2id+"/profile", map[string]any{"name": "越权"}); code != 403 {
		t.Fatalf("member admin PATCH returned %d", code)
	}

	// Deleting a user keeps the member profile and every household record.
	admin.mustOK("DELETE", "/api/v1/admin/users/"+m2id, nil)
	if code, _ := m2.do("GET", "/api/v1/auth/me", nil); code != 401 {
		t.Fatalf("deleted user session alive: %d", code)
	}
	jar2, _ := cookiejar.New(nil)
	gone := &testClient{t: t, base: ts.URL, hc: &http.Client{Jar: jar2}}
	if code, _ := gone.do("POST", "/api/v1/auth/session", map[string]string{"username": "managed-two", "password": "reset-pass-0000"}); code != 401 {
		t.Fatalf("deleted user can log in: %d", code)
	}
	members := v012Rows(t, admin.mustOK("GET", "/api/v1/household/members", nil))
	kept := false
	for _, m := range members {
		if m["name"] == "被管理者" {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("member profile lost after user deletion: %v", members)
	}
	rows := v012Rows(t, admin.mustOK("GET", "/api/v1/transactions?limit=200", nil))
	found := false
	for _, tr := range rows {
		if tr["note"] == "删除前记录" {
			found = true
		}
	}
	if !found {
		t.Fatalf("transaction lost after user deletion: %v", rows)
	}

	// Audit trail recorded the management actions.
	logs := v012Rows(t, admin.mustOK("GET", "/api/v1/admin/audit-logs?limit=100", nil))
	actions := map[string]bool{}
	for _, l := range logs {
		actions[l["action"].(string)] = true
	}
	for _, want := range []string{"update_user_profile", "password_security_reset", "delete_user"} {
		if !actions[want] {
			t.Fatalf("audit log missing %s: %v", want, actions)
		}
	}
}
