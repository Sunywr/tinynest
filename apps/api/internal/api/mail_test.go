package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMailConfigurationRequiresSiteAdmin(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	for _, method := range []string{"GET", "POST"} {
		code, _ := c.do(method, "/api/v1/site/mail", map[string]any{})
		if code != 403 {
			t.Fatalf("household admin %s mail: got %d want 403", method, code)
		}
	}
}

// A deterministic challenge fixture for tests whose subject is account approval,
// not delivery. Security tests below exercise the public challenge endpoint.
func (c *testClient) fixtureProof(owner, email, purpose string) mailProof {
	c.t.Helper()
	id := newID("fixture")
	code := "123456"
	a := c.mailAPI
	if _, e := a.DB.Exec(`INSERT INTO email_challenges(id,owner,email,purpose,digest,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, id, owner, email, purpose, a.digestCode(id, code), a.nowUTC(), a.now().UTC().Add(time.Minute*10).Format(time.RFC3339)); e != nil {
		c.t.Fatal(e)
	}
	return mailProof{ChallengeID: id, Code: code}
}
func (c *testClient) registerVerified(username, password string) {
	email := username + "@example.com"
	proof := c.fixtureProof("registration:"+email, email, "register")
	c.mustOK("POST", "/api/v1/auth/register", map[string]any{"username": username, "password": password, "email": email, "proof": proof})
}
func (c *testClient) changePasswordVerified(current, next string) {
	me := c.mustOK("GET", "/api/v1/auth/me", nil)
	id := me["user_id"].(string)
	email := me["email"].(string)
	proof := c.fixtureProof(id, email, "password")
	c.mustOK("POST", "/api/v1/auth/me/password", map[string]any{"current_password": current, "new_password": next, "proof": proof})
}
func TestMailChallengeReplayAndEncryption(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	config := defaultMailConfig()
	config.Enabled = true
	raw, _ := json.Marshal(config)
	encrypted, _ := a.mailCrypt("smtp-secret", false)
	if _, e := d.Exec(`INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), encrypted, a.nowUTC()); e != nil {
		t.Fatal(e)
	}
	r := c.mustOK("POST", "/api/v1/auth/mail/challenge", map[string]string{"email": "new@example.com", "purpose": "register"})
	id := r["challenge_id"].(string)
	var payload string
	if e := d.QueryRow(`SELECT payload FROM mail_queue WHERE recipient='new@example.com'`).Scan(&payload); e != nil {
		t.Fatal(e)
	}
	plain, e := a.mailCrypt(payload, true)
	if e != nil {
		t.Fatal(e)
	}
	var msg []string
	_ = json.Unmarshal([]byte(plain), &msg)
	code := strings.Split(strings.TrimPrefix(msg[1], "验证码："), "，")[0]
	if strings.Contains(payload, code) {
		t.Fatal("plaintext code in queue")
	}
	proof := mailProof{ChallengeID: id, Code: code}
	if e = a.checkProof(context.Background(), proof, "registration:new@example.com", "register", "new@example.com"); e != nil {
		t.Fatal(e)
	}
	if e = a.consumeProof(context.Background(), d, proof, "registration:new@example.com", "register", "new@example.com"); e != nil {
		t.Fatal(e)
	}
	if e = a.checkProof(context.Background(), proof, "registration:new@example.com", "register", "new@example.com"); e == nil {
		t.Fatal("replay accepted")
	}
}
func TestMailConfigAtomicReplacementAndRedaction(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	id := c.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	if _, e := d.Exec(`INSERT INTO site_administrators(user_id) VALUES(?)`, id); e != nil {
		t.Fatal(e)
	}
	sent := 0
	a.opts.MailSender = func(_ context.Context, _ MailConfig, password, to, subject, body string) error {
		sent++
		if password != "smtp-secret" && password != "new-secret" {
			t.Fatal("wrong SMTP credential")
		}
		return nil
	}
	cfg := defaultMailConfig()
	cfg.Enabled = true
	body := map[string]any{"config": cfg, "password": "smtp-secret", "current_password": "correct-horse-battery", "test_recipient": "test@example.com", "version": 0}
	c.mustOK("POST", "/api/v1/site/mail", body)
	if sent != 1 {
		t.Fatal("no test message")
	}
	got := c.mustOK("GET", "/api/v1/site/mail", nil)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "smtp-secret") || got["password_configured"] != true {
		t.Fatal("credential disclosure")
	}
	body["version"] = 1
	body["password"] = "new-secret"
	a.opts.MailSender = func(context.Context, MailConfig, string, string, string, string) error {
		return errors.New("provider secret must not leak")
	}
	if status, r := c.do("POST", "/api/v1/site/mail", body); status != 502 || strings.Contains(fmt.Sprint(r), "provider secret") {
		t.Fatalf("failed SMTP response: %d %v", status, r)
	}
	_, encrypted, v, e := a.mailConfig(context.Background())
	if e != nil || v != 1 {
		t.Fatal("failed candidate replaced active version")
	}
	password, _ := a.mailCrypt(encrypted, true)
	if password != "smtp-secret" {
		t.Fatal("failed candidate replaced password")
	}
	if status, _ := c.do("POST", "/api/v1/site/mail", map[string]any{"config": cfg, "current_password": "wrong", "test_recipient": "test@example.com", "version": 1}); status != 400 {
		t.Fatal("reauth bypass")
	}
}
func TestMailProofAttemptsExpiryAndPurpose(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	ctx := context.Background()
	proof := c.fixtureProof("test-owner", "test@example.com", "bind")
	if a.checkProof(ctx, proof, "test-owner", "password", "test@example.com") == nil {
		t.Fatal("purpose substitution")
	}
	bad := proof
	bad.Code = "999999"
	for i := 0; i < 5; i++ {
		if a.checkProof(ctx, bad, "test-owner", "bind", "test@example.com") == nil {
			t.Fatal("bad code accepted")
		}
	}
	if a.checkProof(ctx, proof, "test-owner", "bind", "test@example.com") == nil {
		t.Fatal("exhausted challenge accepted")
	}
	proof = c.fixtureProof("test-owner", "test@example.com", "bind")
	_, _ = d.Exec(`UPDATE email_challenges SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, proof.ChallengeID)
	if a.checkProof(ctx, proof, "test-owner", "bind", "test@example.com") == nil {
		t.Fatal("expired challenge accepted")
	}
}
func TestMailQuotaConcurrentAndRestart(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	cfg := defaultMailConfig()
	cfg.DailyLimit = 5
	cfg.AnnouncementLimit = 2
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.reserveMail(context.Background(), "announcement", cfg) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 2 {
		t.Fatalf("announcement quota %d", accepted.Load())
	}
	restarted := New(d, a.opts)
	for i := 0; i < 3; i++ {
		if e := restarted.reserveMail(context.Background(), "verification", cfg); e != nil {
			t.Fatal(e)
		}
	}
	if restarted.reserveMail(context.Background(), "verification", cfg) == nil {
		t.Fatal("total limit exceeded")
	}
}
func TestMailBindResetAndSessionRevocation(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	id := c.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	proof := c.fixtureProof(id, "admin@example.com", "bind")
	c.mustOK("POST", "/api/v1/auth/me/mail/confirm", map[string]any{"email": "admin@example.com", "current_password": "correct-horse-battery", "proof": proof})
	if status, _ := c.do("PATCH", "/api/v1/auth/me/profile", map[string]string{"email": "attacker@example.com"}); status != 400 {
		t.Fatal("profile bypass")
	}
	newProof := c.fixtureProof(id, "new@example.com", "bind")
	if status, _ := c.do("POST", "/api/v1/auth/me/mail/confirm", map[string]any{"email": "new@example.com", "current_password": "correct-horse-battery", "proof": newProof}); status != 400 {
		t.Fatal("old address proof bypass")
	}
	oldProof := c.fixtureProof(id, "admin@example.com", "old_email")
	c.mustOK("POST", "/api/v1/auth/me/mail/confirm", map[string]any{"email": "new@example.com", "current_password": "correct-horse-battery", "proof": newProof, "old_proof": oldProof})
	reset := c.fixtureProof(id, "new@example.com", "reset")
	c.mustOK("POST", "/api/v1/auth/mail/reset", map[string]any{"email": "new@example.com", "new_password": "replacement-password", "proof": reset})
	if status, _ := c.do("GET", "/api/v1/auth/me", nil); status != 401 {
		t.Fatal("session not revoked")
	}
	c.mustOK("POST", "/api/v1/auth/session", map[string]string{"username": "admin", "password": "replacement-password"})
}
func TestMailRegistrationCannotSkipProof(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	if status, _ := c.do("POST", "/api/v1/auth/register", map[string]string{"username": "unverified", "password": "valid-password-123", "email": "u@example.com"}); status != 400 {
		t.Fatal("unverified registration accepted")
	}
}
func TestMailAnnouncementsIsolationPreferenceAndQueue(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	id := c.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	proof := c.fixtureProof(id, "admin@example.com", "bind")
	c.mustOK("POST", "/api/v1/auth/me/mail/confirm", map[string]any{"email": "admin@example.com", "current_password": "correct-horse-battery", "proof": proof})
	cfg := defaultMailConfig()
	cfg.Enabled = true
	raw, _ := json.Marshal(cfg)
	secret, _ := a.mailCrypt("fixture-secret", false)
	if _, e := d.Exec(`INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), secret, a.nowUTC()); e != nil {
		t.Fatal(e)
	}
	foreign := bootstrapHousehold(t, &remEnv{database: d}, "mail-isolation")
	outsider := foreign.client(t, ts.URL)
	// A second household has no matching verified recipient, despite the first one's identity.
	r := outsider.call("POST", "/api/v1/household/mail/announcements", map[string]any{"title": "Hi", "body": "Message", "preview": true}, nil)
	if r.Status != 200 || number(r.Body["recipient_count"]) != 0 {
		t.Fatalf("cross-household recipient leak: %v", r)
	}
	preview := c.mustOK("POST", "/api/v1/household/mail/announcements", map[string]any{"title": "Hi", "body": "Message", "preview": true})
	if number(preview["recipient_count"]) != 1 {
		t.Fatal("verified recipient missing")
	}
	c.mustOK("POST", "/api/v1/household/mail/announcements", map[string]any{"title": "Hi", "body": "Message", "expected_count": 1})
	c.mustOK("POST", "/api/v1/auth/me/mail/preferences", map[string]bool{"announcements": false})
	sends := 0
	a.opts.MailSender = func(context.Context, MailConfig, string, string, string, string) error { sends++; return nil }
	a.processMail(context.Background())
	if sends != 0 {
		t.Fatal("sent after opt-out")
	}
	var state string
	_ = d.QueryRow(`SELECT status FROM mail_queue WHERE kind='announcement'`).Scan(&state)
	if state != "cancelled" {
		t.Fatal(state)
	}
}
func TestMailProofConcurrentConsumption(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	proof := c.fixtureProof("owner", "x@example.com", "bind")
	var wg sync.WaitGroup
	var won atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.mailAPI.consumeProof(context.Background(), d, proof, "owner", "bind", "x@example.com") == nil {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("proof consumed %d times", won.Load())
	}
}
func TestMailQueueRestartRetryAndPayloadCleanup(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	cfg := defaultMailConfig()
	cfg.Enabled = true
	raw, _ := json.Marshal(cfg)
	secret, _ := a.mailCrypt("fixture-secret", false)
	if _, e := d.Exec(`INSERT INTO mail_configuration(id,version,config_json,password_cipher,updated_at) VALUES(1,1,?,?,?)`, string(raw), secret, a.nowUTC()); e != nil {
		t.Fatal(e)
	}
	if e := a.enqueueMail(context.Background(), d, "hh-test", "user-test", "target@example.com", "security", "Security notice", "Body", a.now().UTC().Add(time.Hour).Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	a.opts.MailSender = func(context.Context, MailConfig, string, string, string, string) error {
		return errors.New("temporary outage")
	}
	a.processMail(context.Background())
	var state, payload string
	var attempts int
	if e := d.QueryRow(`SELECT status,payload,attempts FROM mail_queue`).Scan(&state, &payload, &attempts); e != nil {
		t.Fatal(e)
	}
	if state != "queued" || attempts != 1 || payload == "" {
		t.Fatal("retry not persisted")
	}
	_, _ = d.Exec(`UPDATE mail_queue SET due_at='2000-01-01T00:00:00Z'`)
	a.opts.MailSender = func(context.Context, MailConfig, string, string, string, string) error { return nil }
	restart := New(d, a.opts)
	restart.processMail(context.Background())
	_ = d.QueryRow(`SELECT status,payload FROM mail_queue`).Scan(&state, &payload)
	if state != "smtp_accepted" || payload != "" {
		t.Fatal("acceptance or payload cleanup missing")
	}
}
func TestMailAccessAcrossRolesAndCSRF(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	registrationAttempts = newLoginLimiter(LoginLimiterConfig{MaxFailures: 8}, time.Now)
	anon := &testClient{t: t, base: ts.URL, hc: &http.Client{}}
	for _, method := range []string{"GET", "POST"} {
		if status, _ := anon.do(method, "/api/v1/site/mail", map[string]any{}); status != 401 {
			t.Fatal("unauthenticated access")
		}
	}
	member := newMemberClient(t, ts, c, "mail-member", "mail-member-password")
	id := member.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	for _, role := range []string{"member", "viewer"} {
		c.mustOK("PATCH", "/api/v1/admin/users/"+id+"/role", map[string]string{"role": role})
		for _, method := range []string{"GET", "POST"} {
			if status, _ := member.do(method, "/api/v1/site/mail", map[string]any{}); status != 403 {
				t.Fatal(role, " accessed site config")
			}
		}
	}
	adminID := c.mustOK("GET", "/api/v1/auth/me", nil)["user_id"].(string)
	_, _ = d.Exec(`INSERT INTO site_administrators(user_id) VALUES(?)`, adminID)
	wrongCSRF := &testClient{t: t, base: ts.URL, hc: c.hc, csrf: "wrong"}
	if status, _ := wrongCSRF.do("POST", "/api/v1/site/mail", map[string]any{}); status != 403 {
		t.Fatal("CSRF bypass")
	}
	_, _ = d.Exec(`DELETE FROM site_administrators WHERE user_id=?`, adminID)
	if status, _ := c.do("GET", "/api/v1/site/mail", nil); status != 403 {
		t.Fatal("revoked admin retained access")
	}
}
func TestMailExpiredPayloadCleanedWhileDisabled(t *testing.T) {
	ts, c, d := newTestServer(t)
	defer ts.Close()
	defer d.Close()
	a := c.mailAPI
	if e := a.enqueueMail(context.Background(), d, "hh", "owner", "x@example.com", "verification", "Code", "Sensitive body", a.now().UTC().Add(-time.Minute).Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	a.processMail(context.Background())
	var state, payload string
	if e := d.QueryRow(`SELECT status,payload FROM mail_queue`).Scan(&state, &payload); e != nil {
		t.Fatal(e)
	}
	if state != "expired" || payload != "" {
		t.Fatal("disabled service retained expired content")
	}
}
