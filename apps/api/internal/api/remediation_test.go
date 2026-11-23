package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

// testSetupToken is a test-only bootstrap value, never a real credential.
const testSetupToken = "test-only-bootstrap-token-8f3a91c2"

// remEnv is an isolated, empty V2 database + real HTTP server. It never touches
// a real SQLite file.
type remEnv struct {
	t        *testing.T
	ts       *httptest.Server
	mux      *http.ServeMux
	a        *API
	database *db.DB
	opts     Options
}

func newRemEnv(t *testing.T, opts Options) *remEnv {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := database.Migrate(context.Background())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	expected, err := database.ExpectedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(expected) {
		t.Fatalf("expected %d migrations, got %v", len(expected), applied)
	}
	a := New(database, opts)
	mux := server.NewMux(dir, server.Options{Register: a.Register})
	ts := httptest.NewServer(mux)
	registerCaptchaTestServer(t, ts.URL, a)
	t.Cleanup(func() {
		ts.Close()
		_ = database.Close()
	})
	return &remEnv{t: t, ts: ts, mux: mux, a: a, database: database, opts: opts}
}

type remResult struct {
	Status int
	Body   map[string]any
	Header http.Header
}

type remClient struct {
	t       *testing.T
	base    string
	hc      *http.Client
	csrf    string
	headers map[string]string
}

func newRemClient(t *testing.T, base string) *remClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &remClient{t: t, base: base, hc: &http.Client{Jar: jar}}
}

func (c *remClient) call(method, path string, body any, extra map[string]string) remResult {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	if api, ok := captchaTestServers.Load(c.base); ok {
		attachCaptchaForTest(c.t, req, api.(*API))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return remResult{Status: resp.StatusCode, Body: out, Header: resp.Header.Clone()}
}

func setupPayload(name, user string) map[string]any {
	return map[string]any{
		"household_name": name,
		"username":       user,
		"password":       "correct-horse-battery",
	}
}

// --- bookkeeping helpers shared with two-household isolation tests -----------

func (c *remClient) mustCall(method, path string, body any) remResult {
	c.t.Helper()
	result := c.call(method, path, body, nil)
	if result.Status < 200 || result.Status >= 300 {
		c.t.Fatalf("%s %s => %d %v", method, path, result.Status, result.Body)
	}
	return result
}

func (c *remClient) firstCategory(kind string) string {
	c.t.Helper()
	for _, item := range c.mustCall("GET", "/api/v1/categories", nil).Body["items"].([]any) {
		row := item.(map[string]any)
		if row["kind"] == kind {
			return row["id"].(string)
		}
	}
	c.t.Fatalf("no %s category found", kind)
	return ""
}

func (c *remClient) createAccount(kind, name string, opening int64) string {
	c.t.Helper()
	return c.mustCall("POST", "/api/v1/accounts", map[string]any{
		"kind": kind, "name": name, "opening_minor": opening,
	}).Body["id"].(string)
}

func (c *remClient) accountBalance(id string) int64 {
	c.t.Helper()
	return number(c.mustCall("GET", "/api/v1/accounts/"+id, nil).Body["balance_minor"])
}

func (c *remClient) createExpense(amount int64, on, category, account string) string {
	c.t.Helper()
	return c.mustCall("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": amount, "occurred_on": on,
		"category_id": category, "account_id": account,
	}).Body["id"].(string)
}

func (c *remClient) createIncome(amount int64, on, category, account string) string {
	c.t.Helper()
	return c.mustCall("POST", "/api/v1/transactions", map[string]any{
		"type": "income", "amount_minor": amount, "occurred_on": on,
		"category_id": category, "account_id": account,
	}).Body["id"].(string)
}

// ---------------------------------------------------------------------------
// FIX-01: POST /api/v1/setup must require an explicitly configured one-time
// bootstrap token.
// ---------------------------------------------------------------------------

func TestSetupRequiresBootstrapToken(t *testing.T) {
	e := newRemEnv(t, Options{SetupToken: testSetupToken})
	c := newRemClient(t, e.ts.URL)
	valid := map[string]string{"X-Setup-Token": testSetupToken}

	// No token -> rejected, and no household/admin is created.
	if r := c.call("POST", "/api/v1/setup", setupPayload("小家", "admin"), nil); r.Status != 401 && r.Status != 403 {
		t.Fatalf("[FIX-01] unauthenticated setup must be rejected, got %d %v", r.Status, r.Body)
	}
	var households int
	if err := e.database.QueryRow(`SELECT COUNT(*) FROM households`).Scan(&households); err != nil {
		t.Fatal(err)
	}
	if households != 0 {
		t.Fatalf("[FIX-01] rejected setup must not create a household, got %d", households)
	}

	// Wrong token -> rejected.
	wrong := map[string]string{"X-Setup-Token": "wrong-token"}
	if r := c.call("POST", "/api/v1/setup", setupPayload("小家", "admin"), wrong); r.Status != 401 && r.Status != 403 {
		t.Fatalf("[FIX-01] wrong bootstrap token must be rejected, got %d %v", r.Status, r.Body)
	}

	// Correct token -> exactly one successful initialization.
	if r := c.call("POST", "/api/v1/setup", setupPayload("小家", "admin"), valid); r.Status != 201 {
		t.Fatalf("[FIX-01] valid bootstrap token should initialize once, got %d %v", r.Status, r.Body)
	}
	if r := c.call("POST", "/api/v1/setup", setupPayload("另一个家", "admin2"), valid); r.Status == 201 || r.Status == 200 {
		t.Fatalf("[FIX-01] bootstrap token must not create a second admin, got %d", r.Status)
	}
	var admins, users int
	if err := e.database.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if err := e.database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if admins != 1 || users != 1 {
		t.Fatalf("[FIX-01] exactly one admin/user expected, got admins=%d users=%d", admins, users)
	}
}

func TestSetupDisabledWithoutConfiguredToken(t *testing.T) {
	e := newRemEnv(t, Options{})
	c := newRemClient(t, e.ts.URL)
	r := c.call("POST", "/api/v1/setup", setupPayload("小家", "admin"), nil)
	if r.Status == 201 || r.Status == 200 {
		t.Fatalf("[FIX-01] empty database without TINYNEST_SETUP_TOKEN must refuse initialization, got %d", r.Status)
	}
	var n int
	if err := e.database.QueryRow(`SELECT COUNT(*) FROM households`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("[FIX-01] initialization must not happen without a configured token, households=%d", n)
	}
}

func TestSetupConcurrentBootstrapOnlyOneSucceeds(t *testing.T) {
	e := newRemEnv(t, Options{SetupToken: testSetupToken})
	const n = 12
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := &remClient{t: e.t, base: e.ts.URL, hc: &http.Client{}}
			r := c.call("POST", "/api/v1/setup",
				setupPayload("并发家庭", fmt.Sprintf("admin%d", i)),
				map[string]string{"X-Setup-Token": testSetupToken})
			statuses[i] = r.Status
		}(i)
	}
	wg.Wait()
	success := 0
	for _, s := range statuses {
		if s == 201 {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("[FIX-01] concurrent bootstrap must yield exactly one 201, got %d: %v", success, statuses)
	}
	var admins int
	if err := e.database.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Fatalf("[FIX-01] concurrent bootstrap must leave exactly one admin, got %d", admins)
	}
}

// ---------------------------------------------------------------------------
// FIX-02: financial aggregates MUST be scoped to the authenticated household
// inside SQL, not filtered after a global SUM. Two households live in the SAME
// temporary database on purpose and use IDENTICAL ledger subject codes.
// ---------------------------------------------------------------------------

type isoHousehold struct {
	id, user, raw, csrf, username string
}

func bootstrapHousehold(t *testing.T, e *remEnv, suffix string) isoHousehold {
	t.Helper()
	h := isoHousehold{
		id:       "hh_iso_" + suffix,
		user:     "usr_iso_" + suffix,
		raw:      "session-raw-" + suffix,
		csrf:     "csrf-" + suffix,
		username: "admin_" + suffix,
	}
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.Exec(`INSERT INTO households(id,name) VALUES(?,?)`, h.id, "隔离测试家庭"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.Exec(
		`INSERT INTO users(id,household_id,username,password_hash,role) VALUES(?,?,?,?,'admin')`,
		h.user, h.id, h.username, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.Exec(
		`INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES(?,?,?,?,?)`,
		"sess_iso_"+suffix, h.user, auth.HashToken(h.raw), h.csrf, "2099-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	// A real household always has the default single-level category set.
	if err := db.SeedForHousehold(context.Background(), e.database, h.id); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h isoHousehold) client(t *testing.T, base string) *remClient {
	t.Helper()
	c := newRemClient(t, base)
	c.headers = map[string]string{"Cookie": "tn_session=" + h.raw}
	c.csrf = h.csrf
	return c
}

// ---------------------------------------------------------------------------
// FIX-03: every computed business date must use the injectable Asia/Shanghai
// clock. Audit/session timestamps stay UTC.
// ---------------------------------------------------------------------------

func createAccountWithOpening(t *testing.T, e *remEnv, h isoHousehold, amount int64) string {
	t.Helper()
	c := h.client(t, e.ts.URL)
	r := c.call("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "业务日账户", "opening_minor": amount,
	}, nil)
	if r.Status != 201 {
		t.Fatalf("[FIX-03] create account failed: %d %v", r.Status, r.Body)
	}
	id, _ := r.Body["id"].(string)
	if id == "" {
		t.Fatal("[FIX-03] create account returned no id")
	}
	return id
}

func TestBusinessDateOpeningBalanceBoundaries(t *testing.T) {
	cases := []struct {
		name string
		utc  time.Time
		want string
	}{
		{"UTC 00:01 / Beijing 08:01", time.Date(2026, 11, 14, 0, 1, 0, 0, time.UTC), "2026-11-14"},
		{"UTC 16:00 / Beijing next day 00:00", time.Date(2026, 11, 13, 16, 0, 0, 0, time.UTC), "2026-11-14"},
		{"UTC 23:59 / Beijing next day 07:59", time.Date(2026, 11, 13, 23, 59, 0, 0, time.UTC), "2026-11-14"},
		{"month-end rollover", time.Date(2026, 11, 30, 16, 0, 0, 0, time.UTC), "2026-12-01"},
		{"leap day", time.Date(2028, 2, 28, 16, 0, 0, 0, time.UTC), "2028-02-29"},
		{"year rollover", time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC), "2027-01-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := tc.utc
			e := newRemEnv(t, Options{
				Clock:      func() time.Time { return clk },
				SetupToken: testSetupToken,
			})
			h := bootstrapHousehold(t, e, "bd")
			createAccountWithOpening(t, e, h, 10000)
			var occurred string
			if err := e.database.QueryRow(
				`SELECT occurred_on FROM journal_entries WHERE biz_type='opening_cash' AND household_id=?`, h.id).Scan(&occurred); err != nil {
				t.Fatal(err)
			}
			if occurred != tc.want {
				t.Errorf("[FIX-03] opening balance occurred_on=%s, want %s (Asia/Shanghai business day)", occurred, tc.want)
			}
		})
	}
}

func TestBusinessDateDefaultStatsPeriod(t *testing.T) {
	clk := time.Date(2030, 1, 1, 4, 0, 0, 0, time.UTC) // Beijing 2030-01-01 12:00
	e := newRemEnv(t, Options{Clock: func() time.Time { return clk }, SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "period")
	c := h.client(t, e.ts.URL)
	summary := c.call("GET", "/api/v1/stats/summary", nil, nil)
	if summary.Status != 200 {
		t.Fatalf("[FIX-03] stats summary failed: %d %v", summary.Status, summary.Body)
	}
	if summary.Body["from"] != "2030-01-01" || summary.Body["to"] != "2030-01-01" {
		t.Errorf("[FIX-03] default reporting period must follow the Beijing business day, got from=%v to=%v",
			summary.Body["from"], summary.Body["to"])
	}
	monthly := c.call("GET", "/api/v1/stats/monthly?months=1", nil, nil)
	months, _ := monthly.Body["months"].([]any)
	if len(months) != 1 || months[0].(map[string]any)["month"] != "2030-01" {
		t.Errorf("[FIX-03] default monthly window must follow the Beijing business month, got %v", monthly.Body["months"])
	}
}

// ---------------------------------------------------------------------------
// FIX-04: session cookie security must be an explicit deployment decision.
// ---------------------------------------------------------------------------

func loginPost(t *testing.T, e *remEnv, h isoHousehold, extra map[string]string) remResult {
	t.Helper()
	c := h.client(t, e.ts.URL)
	c.csrf = ""
	return c.call("POST", "/api/v1/auth/session", map[string]any{
		"username": h.username, "password": "correct-horse-battery",
	}, extra)
}

func TestSessionCookieSecureInProductionProxy(t *testing.T) {
	e := newRemEnv(t, Options{CookieSecure: true, SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "sec")
	r := loginPost(t, e, h, map[string]string{"X-Forwarded-Proto": "https"})
	if r.Status != 200 {
		t.Fatalf("[FIX-04] login failed: %d %v", r.Status, r.Body)
	}
	cookie := r.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "Secure") {
		t.Errorf("[FIX-04] production cookie must be Secure even when Go receives plain HTTP behind a TLS terminator, got: %s", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Errorf("[FIX-04] HttpOnly/SameSite must not regress, got: %s", cookie)
	}
	if csrf, _ := r.Body["csrf_token"].(string); csrf == "" {
		t.Error("[FIX-04] login must still return a CSRF token")
	}
}

func TestSessionCookieSecureCanBeDisabledForLocalDev(t *testing.T) {
	e := newRemEnv(t, Options{CookieSecure: false, SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "dev")
	r := loginPost(t, e, h, nil)
	if r.Status != 200 {
		t.Fatalf("[FIX-04] login failed: %d %v", r.Status, r.Body)
	}
	if strings.Contains(r.Header.Get("Set-Cookie"), "Secure") {
		t.Errorf("[FIX-04] explicitly disabled Secure must be honored for local plain HTTP, got: %s",
			r.Header.Get("Set-Cookie"))
	}
}

func TestSessionCookieHonorsTrustedProxyForwardedProto(t *testing.T) {
	e := newRemEnv(t, Options{
		CookieSecure:   false,
		TrustedProxies: []string{"127.0.0.1/32"},
		SetupToken:     testSetupToken,
	})
	h := bootstrapHousehold(t, e, "trc")
	r := loginPost(t, e, h, map[string]string{"X-Forwarded-Proto": "https"})
	if r.Status != 200 {
		t.Fatalf("[FIX-04] login failed: %d %v", r.Status, r.Body)
	}
	if !strings.Contains(r.Header.Get("Set-Cookie"), "Secure") {
		t.Errorf("[FIX-04] X-Forwarded-Proto=https from a configured trusted proxy must yield a Secure cookie, got: %s",
			r.Header.Get("Set-Cookie"))
	}
}

func TestSessionCookieIgnoresUntrustedForwardedProto(t *testing.T) {
	e := newRemEnv(t, Options{
		CookieSecure:   false,
		TrustedProxies: []string{"10.0.0.0/8"},
		SetupToken:     testSetupToken,
	})
	h := bootstrapHousehold(t, e, "unt")
	r := loginPost(t, e, h, map[string]string{"X-Forwarded-Proto": "https"})
	if r.Status != 200 {
		t.Fatalf("[FIX-04] login failed: %d %v", r.Status, r.Body)
	}
	if strings.Contains(r.Header.Get("Set-Cookie"), "Secure") {
		t.Errorf("[FIX-04] an untrusted client must not influence cookie security with a forged X-Forwarded-Proto header, got: %s",
			r.Header.Get("Set-Cookie"))
	}
}

// ---------------------------------------------------------------------------
// FIX-05: failed-login throttling must be configurable, testable, bounded in
// time, concurrency-safe and must never log passwords or tokens.
// ---------------------------------------------------------------------------

// serve issues a request with a controllable RemoteAddr so per-IP buckets can
// be exercised without binding extra listeners.
func (e *remEnv) serve(method, path string, body any, remoteAddr string, extra map[string]string) remResult {
	return e.serveRequest(method, path, body, remoteAddr, extra, true)
}

func (e *remEnv) serveRaw(method, path string, body any, remoteAddr string, extra map[string]string) remResult {
	return e.serveRequest(method, path, body, remoteAddr, extra, false)
}

func (e *remEnv) serveRequest(method, path string, body any, remoteAddr string, extra map[string]string, captcha bool) remResult {
	e.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = remoteAddr
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	if captcha {
		attachCaptchaForTest(e.t, req, e.a)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return remResult{Status: res.StatusCode, Body: out, Header: res.Header.Clone()}
}

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newRateLimitEnv(t *testing.T, cfg LoginLimiterConfig, clk *mutableClock) *remEnv {
	return newRemEnv(t, Options{
		Clock:        clk.Now,
		LoginLimiter: cfg,
		SetupToken:   testSetupToken,
	})
}

func wrongLogin(user string) map[string]any {
	return map[string]any{"username": user, "password": "definitely-wrong-password"}
}

func correctLogin(user string) map[string]any {
	return map[string]any{"username": user, "password": "correct-horse-battery"}
}

func testLimiterConfig() LoginLimiterConfig {
	return LoginLimiterConfig{MaxFailures: 3, Window: 60 * time.Second, Lockout: 30 * time.Second}
}

func TestLoginFailureRateLimit(t *testing.T) {
	clk := &mutableClock{now: time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)}
	cfg := testLimiterConfig()
	e := newRateLimitEnv(t, cfg, clk)
	h := bootstrapHousehold(t, e, "lim")
	ip := "203.0.113.10:5000"

	for i := 0; i < cfg.MaxFailures; i++ {
		r := e.serve("POST", "/api/v1/auth/session", wrongLogin(h.username), ip, nil)
		if r.Status != 401 {
			t.Fatalf("[FIX-05] failed login #%d should be 401, got %d %v", i+1, r.Status, r.Body)
		}
	}
	blocked := e.serve("POST", "/api/v1/auth/session", wrongLogin(h.username), ip, nil)
	if blocked.Status != 429 {
		t.Fatalf("[FIX-05] login must return 429 after %d consecutive failures, got %d %v",
			cfg.MaxFailures, blocked.Status, blocked.Body)
	}
	if blocked.Header.Get("Retry-After") == "" {
		t.Error("[FIX-05] a 429 response must carry a Retry-After hint")
	}

	// Cooldown is bounded: the legitimate owner must never be locked out forever.
	clk.Advance(cfg.Lockout + time.Second)
	ok := e.serve("POST", "/api/v1/auth/session", correctLogin(h.username), ip, nil)
	if ok.Status != 200 {
		t.Fatalf("[FIX-05] correct credentials must work again after cooldown, got %d %v", ok.Status, ok.Body)
	}
	if csrf, _ := ok.Body["csrf_token"].(string); csrf == "" {
		t.Error("[FIX-05] successful login must still issue a CSRF token")
	}
}

func TestLoginRateLimitScopeIsolation(t *testing.T) {
	clk := &mutableClock{now: time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)}
	cfg := testLimiterConfig()
	e := newRateLimitEnv(t, cfg, clk)
	h1 := bootstrapHousehold(t, e, "l1")
	h2 := bootstrapHousehold(t, e, "l2")
	ip1, ip2, ip3 := "198.51.100.7:4000", "198.51.100.8:4000", "198.51.100.9:4000"

	for i := 0; i < cfg.MaxFailures; i++ {
		e.serve("POST", "/api/v1/auth/session", wrongLogin(h1.username), ip1, nil)
	}
	if r := e.serve("POST", "/api/v1/auth/session", wrongLogin(h1.username), ip1, nil); r.Status != 429 {
		t.Errorf("[FIX-05] exhausted principal/IP bucket should be 429, got %d", r.Status)
	}
	if r := e.serve("POST", "/api/v1/auth/session", correctLogin(h2.username), ip2, nil); r.Status != 200 {
		t.Errorf("[FIX-05] unrelated IP and account must not be blocked, got %d %v", r.Status, r.Body)
	}

	// Auth failures must not disclose whether a username exists.
	unknown := e.serve("POST", "/api/v1/auth/session", wrongLogin("no-such-user"), ip3, nil)
	known := e.serve("POST", "/api/v1/auth/session", wrongLogin(h2.username), ip3, nil)
	if fmt.Sprintf("%v", unknown.Body["error"]) != fmt.Sprintf("%v", known.Body["error"]) {
		t.Errorf("[FIX-05] failure response must not leak username existence: unknown=%v known=%v",
			unknown.Body["error"], known.Body["error"])
	}
}

func TestLoginRateLimitConcurrentAndAuditSafe(t *testing.T) {
	clk := &mutableClock{now: time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)}
	e := newRateLimitEnv(t, testLimiterConfig(), clk)
	h := bootstrapHousehold(t, e, "cnc")

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	ip := "203.0.113.77:6000"
	const n = 20
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := e.serve("POST", "/api/v1/auth/session", wrongLogin(h.username), ip, nil)
			statuses[i] = r.Status
		}(i)
	}
	wg.Wait()
	blocked := 0
	for _, s := range statuses {
		if s == 429 {
			blocked++
		}
	}
	if blocked == 0 {
		t.Errorf("[FIX-05] concurrent failures must eventually be throttled, statuses=%v", statuses)
	}
	logs := buf.String()
	if strings.Contains(logs, "definitely-wrong-password") || strings.Contains(logs, "correct-horse-battery") {
		t.Error("[FIX-05] security audit events must never contain passwords")
	}
	if strings.Contains(logs, "tn_session=") {
		t.Error("[FIX-05] security audit events must never contain session tokens")
	}
}
