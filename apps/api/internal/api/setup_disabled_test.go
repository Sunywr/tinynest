package api

import (
	"sync"
	"testing"
	"time"
)

// Production has already been initialized. An explicit operator switch must be
// able to close the bootstrap endpoint entirely, while a fresh offline deploy
// (switch off + one-time token) keeps the controlled bootstrap path working.

func errorCode(t *testing.T, r remResult) string {
	t.Helper()
	m, _ := r.Body["error"].(map[string]any)
	code, _ := m["code"].(string)
	return code
}

func countRows(t *testing.T, e *remEnv, table string) int {
	t.Helper()
	var n int
	if err := e.database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSetupDisabledFlagRefusesBootstrapEvenWithValidToken(t *testing.T) {
	now := time.Date(2026, 11, 14, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{
		Clock:         func() time.Time { return now },
		SetupToken:    testSetupToken,
		SetupDisabled: true,
	})
	c := newRemClient(t, e.ts.URL)
	body := map[string]any{
		"household_name": "不应被创建", "username": "admin", "password": "correct-horse-battery",
	}
	headers := map[string]string{setupTokenHeader: testSetupToken}

	first := c.call("POST", "/api/v1/setup", body, headers)
	if first.Status != 403 || errorCode(t, first) != "setup_disabled" {
		t.Fatalf("disabled setup must return 403 setup_disabled, got %d %v", first.Status, first.Body)
	}
	// Unauthenticated and wrong-token callers get the same closed endpoint.
	anonymous := c.call("POST", "/api/v1/setup", body, nil)
	wrong := c.call("POST", "/api/v1/setup", body, map[string]string{setupTokenHeader: "wrong"})
	if anonymous.Status != 403 || wrong.Status != 403 {
		t.Errorf("disabled setup must reject every caller: anon=%d wrong=%d", anonymous.Status, wrong.Status)
	}

	// 20 concurrent attempts must never create a household, admin or seed data.
	var wg sync.WaitGroup
	statuses := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- c.call("POST", "/api/v1/setup", body, headers).Status
		}()
	}
	wg.Wait()
	close(statuses)
	for s := range statuses {
		if s != 403 {
			t.Errorf("concurrent setup returned %d, want 403", s)
		}
	}
	if got := countRows(t, e, "households"); got != 0 {
		t.Errorf("households created while setup disabled: %d", got)
	}
	if got := countRows(t, e, "users"); got != 0 {
		t.Errorf("admins created while setup disabled: %d", got)
	}
	if got := countRows(t, e, "categories"); got != 0 {
		t.Errorf("seed data written while setup disabled: %d", got)
	}
}

func TestSetupDisabledFlagOffKeepsControlledBootstrap(t *testing.T) {
	now := time.Date(2026, 11, 14, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{
		Clock:         func() time.Time { return now },
		SetupToken:    testSetupToken,
		SetupDisabled: false,
	})
	c := newRemClient(t, e.ts.URL)
	body := map[string]any{
		"household_name": "受控首次部署", "username": "admin", "password": "correct-horse-battery",
	}
	headers := map[string]string{setupTokenHeader: testSetupToken}

	first := c.call("POST", "/api/v1/setup", body, headers)
	if first.Status != 201 {
		t.Fatalf("controlled bootstrap must still work on a fresh DB: %d %v", first.Status, first.Body)
	}
	second := c.call("POST", "/api/v1/setup", body, headers)
	if second.Status != 409 {
		t.Errorf("second bootstrap must be refused: %d %v", second.Status, second.Body)
	}
	if got := countRows(t, e, "households"); got != 1 {
		t.Errorf("households = %d, want 1", got)
	}
	if got := countRows(t, e, "users"); got != 1 {
		t.Errorf("admins = %d, want 1", got)
	}
	// A token-less empty database still fails closed when the switch is off.
	empty := newRemEnv(t, Options{Clock: func() time.Time { return now }})
	emptyClient := newRemClient(t, empty.ts.URL)
	noToken := emptyClient.call("POST", "/api/v1/setup", body, nil)
	if noToken.Status != 403 || errorCode(t, noToken) != "setup_disabled" {
		t.Errorf("empty DB without token must fail closed: %d %v", noToken.Status, noToken.Body)
	}
	if got := countRows(t, empty, "users"); got != 0 {
		t.Errorf("token-less setup created an admin: %d", got)
	}
}
