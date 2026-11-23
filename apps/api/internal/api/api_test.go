package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tinynest/api/internal/db"
	"tinynest/api/internal/server"
)

type testClient struct {
	mailAPI *API
	t       testing.TB
	base    string
	hc      *http.Client
	csrf    string
	headers map[string]string
}

// newTestServer boots an isolated database + real HTTP server, runs the one-time
// setup and logs in as the admin. It returns the SQLite handle for fixture checks.
func newTestServer(t testing.TB) (*httptest.Server, *testClient, *db.DB) {
	return newTestServerWithOptions(t, Options{MailKey: bytes.Repeat([]byte{7}, 32)})
}

// newTestServerWithOptions lets a test inject a clock, cookie policy or limiter
// before the bootstrap request runs.
func newTestServerWithOptions(t testing.TB, opts Options) (*httptest.Server, *testClient, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	var database *db.DB
	var err error
	if socket := os.Getenv("TINYNEST_TEST_MYSQL_SOCKET"); socket != "" {
		password, readErr := os.ReadFile(os.Getenv("TINYNEST_TEST_MYSQL_PASSWORD_FILE"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		database, err = db.OpenMySQL(socket, os.Getenv("TINYNEST_TEST_MYSQL_DATABASE"), os.Getenv("TINYNEST_TEST_MYSQL_USER"), string(bytes.TrimSpace(password)))
	} else {
		database, err = db.Open(filepath.Join(dir, "test.sqlite3"))
	}
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
	opts.SetupToken = testSetupToken
	a := New(database, opts)
	mux := server.NewMux(dir, server.Options{Register: a.Register})
	ts := httptest.NewServer(mux)
	registerCaptchaTestServer(t, ts.URL, a)
	jar, _ := cookiejar.New(nil)
	// The one-time bootstrap token is used ONLY for the initial setup call.
	tc := &testClient{mailAPI: a, t: t, base: ts.URL, hc: &http.Client{Jar: jar},
		headers: map[string]string{setupTokenHeader: testSetupToken}}

	code, body := tc.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "测试小家", "username": "admin", "password": "correct-horse-battery",
	})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, body)
	}
	tc.headers = nil
	code, body = tc.do("POST", "/api/v1/auth/session", map[string]any{
		"username": "admin", "password": "correct-horse-battery",
	})
	if code != 200 {
		t.Fatalf("login: %d %v", code, body)
	}
	tc.csrf, _ = body["csrf_token"].(string)
	if tc.csrf == "" {
		t.Fatal("missing csrf token")
	}
	return ts, tc, database
}

func (c *testClient) do(method, path string, body any) (int, map[string]any) {
	return c.doHeaders(method, path, body, nil)
}

func (c *testClient) doHeaders(method, path string, body any, headers map[string]string) (int, map[string]any) {
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
	for k, v := range headers {
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
	return resp.StatusCode, out
}

func number(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func (c *testClient) mustOK(method, path string, body any) map[string]any {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if code < 200 || code >= 300 {
		c.t.Fatalf("%s %s => %d %v", method, path, code, out)
	}
	return out
}

func (c *testClient) firstCategory(kind string) string {
	c.t.Helper()
	_, out := c.do("GET", "/api/v1/categories", nil)
	items, _ := out["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["kind"] == kind {
			return m["id"].(string)
		}
	}
	c.t.Fatalf("no %s category found", kind)
	return ""
}

// v012Rows flattens an {"items": [...]} response into typed rows.
func v012Rows(t testing.TB, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item is not an object: %v", item)
		}
		out = append(out, row)
	}
	return out
}

// findRow returns the first row with column == value (or fails the test).
func findRow(t testing.TB, body map[string]any, column, value string) map[string]any {
	t.Helper()
	for _, row := range v012Rows(t, body) {
		if row[column] == value {
			return row
		}
	}
	t.Fatalf("no row with %s=%s in %v", column, value, body)
	return nil
}

// TestSetupIsSingleUseAndSessionAuthEnforced covers the post-FIX-01 contract:
// the endpoint is single-use AND bootstrap-protected; ordinary reads still need
// a session and writes still need CSRF.
func TestSetupIsSingleUseAndSessionAuthEnforced(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// A second, correctly tokened bootstrap must be rejected (no second admin).
	c.headers = map[string]string{setupTokenHeader: testSetupToken}
	code, _ := c.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "另一个家", "username": "admin2", "password": "another-password",
	})
	if code != 409 {
		t.Errorf("duplicate setup should be 409, got %d", code)
	}
	c.headers = nil
	var admins int
	_ = database.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&admins)
	if admins != 1 {
		t.Errorf("duplicate setup must not create a second admin, got %d", admins)
	}

	// A tokenless bootstrap attempt is refused outright.
	noToken := &testClient{t: t, base: c.base, hc: &http.Client{}}
	code, _ = noToken.do("POST", "/api/v1/setup", map[string]any{
		"household_name": "抢注家庭", "username": "attacker", "password": "another-password",
	})
	if code != 403 && code != 401 {
		t.Errorf("tokenless setup should be rejected, got %d", code)
	}

	// A fresh client without a session cookie cannot read financial data.
	fresh := &testClient{t: t, base: c.base, hc: &http.Client{}}
	code, _ = fresh.do("GET", "/api/v1/accounts", nil)
	if code != 401 {
		t.Errorf("unauthenticated read should be 401, got %d", code)
	}
	// CSRF is enforced on writes even with a valid session.
	noCSRF := &testClient{t: t, base: c.base, hc: c.hc, csrf: "wrong-token"}
	code, _ = noCSRF.do("POST", "/api/v1/categories", map[string]any{"kind": "expense", "name": "x"})
	if code != 403 {
		t.Errorf("bad CSRF should be 403, got %d", code)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	ts, _, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	applied, err := database.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Errorf("re-running migrations should be a no-op, got %v", applied)
	}
	var tables int
	if database.IsMySQL() {
		err = database.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()`).Scan(&tables)
	} else {
		err = database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables)
	}
	if err != nil {
		t.Fatal(err)
	}
	// 21 core + 4 bookkeeping + schema_migrations = 26 tables. SQLite also adds
	// sqlite_sequence because of AUTOINCREMENT, so the check is a lower bound.
	if tables < 26 {
		t.Errorf("expected the full V2 schema, got %d tables", tables)
	}
	var baseline string
	if err := database.QueryRow(`SELECT value FROM schema_meta WHERE meta_key='baseline'`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != "v2" {
		t.Errorf("schema_meta.baseline = %q, want v2", baseline)
	}
}
