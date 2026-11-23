package api

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestRegistrationCanSignInBeforeVerification(t *testing.T) {
	server, admin, database := newTestServer(t)
	defer database.Close()
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	c := &testClient{t: t, base: server.URL, hc: &http.Client{Jar: jar}, mailAPI: admin.mailAPI}
	body := map[string]string{"username": "onboarding_test", "password": "long-test-password"}
	c.mustOK("POST", "/api/v1/auth/register", body)
	c.csrf = c.mustOK("POST", "/api/v1/auth/session", body)["csrf_token"].(string)
	me := c.mustOK("GET", "/api/v1/auth/me", nil)
	if me["household_id"] != "" || me["email_verified"] != false {
		t.Fatalf("unexpected initial access: %v", me)
	}
	if status, _ := c.do("GET", "/api/v1/household/current", nil); status != 403 {
		t.Fatalf("household status = %d", status)
	}
	if status, _ := c.do("POST", "/api/v1/household/current", map[string]string{"name": "测试家庭"}); status != 403 {
		t.Fatalf("create before verification status = %d", status)
	}

	email := "onboarding@example.com"
	proof := c.fixtureProof(me["user_id"].(string), email, "bind")
	c.mustOK("POST", "/api/v1/auth/me/mail/confirm", map[string]any{
		"email": email, "current_password": body["password"], "proof": proof,
	})
	me = c.mustOK("GET", "/api/v1/auth/me", nil)
	if me["email_verified"] != true || me["household_id"] != "" {
		t.Fatalf("unexpected verified access: %v", me)
	}
	created := c.mustOK("POST", "/api/v1/household/current", map[string]string{"name": "测试家庭"})
	me = c.mustOK("GET", "/api/v1/auth/me", nil)
	if me["household_id"] != created["household_id"] || me["role"] != "admin" {
		t.Fatalf("unexpected household access: %v", me)
	}
	c.mustOK("GET", "/api/v1/household/current", nil)
	invite := c.mustOK("POST", "/api/v1/household/invitations", map[string]string{"member_name": "新成员"})
	owner := c
	joinJar, _ := cookiejar.New(nil)
	c = &testClient{t: t, base: server.URL, hc: &http.Client{Jar: joinJar}, mailAPI: admin.mailAPI}
	body["username"] = "onboarding_join"
	c.mustOK("POST", "/api/v1/auth/register", body)
	c.csrf = c.mustOK("POST", "/api/v1/auth/session", body)["csrf_token"].(string)
	join := map[string]any{"code": invite["code"]}
	if status, _ := c.do("POST", "/api/v1/auth/join", join); status != 403 {
		t.Fatalf("join before verification status = %d", status)
	}
	me = c.mustOK("GET", "/api/v1/auth/me", nil)
	email = "onboarding_join@example.com"
	c.mustOK("POST", "/api/v1/auth/me/mail/confirm", map[string]any{
		"email": email, "current_password": body["password"], "proof": c.fixtureProof(me["user_id"].(string), email, "bind"),
	})
	c.mustOK("POST", "/api/v1/auth/join", join)
	c.mustOK("POST", "/api/v1/auth/join", join)
	if me := c.mustOK("GET", "/api/v1/auth/me", nil); me["join_pending"] != true {
		t.Fatalf("missing pending application: %v", me)
	}
	if status, _ := c.do("GET", "/api/v1/household/current", nil); status != 403 {
		t.Fatalf("household before approval status = %d", status)
	}
	applications := owner.mustOK("GET", "/api/v1/admin/join-applications", nil)
	id := applications["items"].([]any)[0].(map[string]any)["id"].(string)
	owner.mustOK("POST", "/api/v1/admin/join-applications/"+id, map[string]bool{"approve": true})
	me = c.mustOK("GET", "/api/v1/auth/me", nil)
	if me["household_id"] != created["household_id"] || me["role"] != "member" || me["member_id"] == "" {
		t.Fatalf("unexpected approved access: %v", me)
	}
}
