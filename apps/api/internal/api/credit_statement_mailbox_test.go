package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStatementMailboxReadOnlyProbe(t *testing.T) {
	const email = "123@qq.com"
	const code = `dummy"code\value`
	for _, tc := range []struct {
		name, responses, commands string
		ok                        bool
	}{
		{"read-only inbox", "* OK ready\r\nA1 OK logged in\r\n* 3 EXISTS\r\nA2 OK [READ-ONLY] selected\r\n", "A1 LOGIN \"123@qq.com\" \"dummy\\\"code\\\\value\"\r\nA2 EXAMINE INBOX\r\n", true},
		{"rejected credentials", "* OK ready\r\nA1 NO dummy credential rejected\r\n", "A1 LOGIN \"123@qq.com\" \"dummy\\\"code\\\\value\"\r\n", false},
		{"inbox rejected", "* OK ready\r\nA1 OK logged in\r\nA2 NO inbox unavailable\r\n", "A1 LOGIN \"123@qq.com\" \"dummy\\\"code\\\\value\"\r\nA2 EXAMINE INBOX\r\n", false},
		{"malformed greeting", "* OKAY ready\r\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := struct {
				*strings.Reader
				strings.Builder
			}{Reader: strings.NewReader(tc.responses)}
			err := examineStatementInbox(&wire, email, code)
			if (err == nil) != tc.ok || wire.String() != tc.commands {
				t.Fatalf("probe result %v, commands %q", err, wire.String())
			}
			if err != nil && strings.Contains(err.Error(), "dummy") {
				t.Fatal("server response exposed")
			}
		})
	}
}

func TestStatementMailbox163IdentifiesClientBeforeExamine(t *testing.T) {
	wire := struct {
		*strings.Reader
		strings.Builder
	}{Reader: strings.NewReader("* OK ready\r\nA1 OK logged in\r\n* ID NIL\r\nA2 OK identified\r\nA3 OK [READ-ONLY] opened\r\n")}
	if err := examineStatementInbox(&wire, "fixture@163.com", "fixture"); err != nil {
		t.Fatal(err)
	}
	want := "A1 LOGIN \"fixture@163.com\" \"fixture\"\r\nA2 ID (\"name\" \"TinyNest\" \"version\" \"2\" \"vendor\" \"TinyNest\")\r\nA3 EXAMINE INBOX\r\n"
	if wire.String() != want {
		t.Fatalf("unexpected wire commands: %q", wire.String())
	}
}

func TestStatementMailboxConfigurationLifecycle(t *testing.T) {
	const path = "/api/v1/credit-statement-mailbox"
	const address = "123456@qq.com"
	const firstCode = "first-fixture-code"
	const rotatedCode = "rotated-fixture-code"
	var tested atomic.Value
	ts, client, database := newTestServerWithOptions(t, Options{
		MailKey: bytes.Repeat([]byte{7}, 32),
		StatementMailboxProbe: func(_ context.Context, email, code string) error {
			if email != address || code == "rejected" {
				return errors.New("invalid mailbox")
			}
			tested.Store(code)
			return nil
		},
	})
	defer ts.Close()
	defer database.Close()

	store := func(version int, code string) (int, map[string]any) {
		return client.do("PUT", path, map[string]any{"version": version, "email": address, "auth_code": code})
	}
	checkConfig := func(config map[string]any, version int, enabled bool) {
		t.Helper()
		if config["version"] != float64(version) || config["enabled"] != enabled ||
			config["email"] != nil || config["auth_code"] != nil || config["auth_code_cipher"] != nil {
			t.Fatalf("unexpected config response: %v", config)
		}
		if enabled && config["masked_email"] != "1***@qq.com" {
			t.Fatalf("email should be masked: %v", config)
		}
	}
	checkConfig(client.mustOK("GET", path, nil), 0, false)
	status, config := store(0, firstCode)
	if status != http.StatusOK {
		t.Fatalf("save got %d: %v", status, config)
	}
	checkConfig(config, 1, true)
	user := client.mustOK("GET", "/api/v1/auth/me", nil)
	if _, err := database.Exec(`INSERT INTO households(id,name) VALUES('hh_mailbox_other','第二家庭')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE users SET household_id='hh_mailbox_other' WHERE id=?`, user["user_id"]); err != nil {
		t.Fatal(err)
	}
	checkConfig(client.mustOK("GET", path, nil), 0, false)
	if status, _ := client.do("POST", path+"/test", map[string]int{"version": 1}); status != http.StatusConflict {
		t.Fatalf("different household tested an old configuration: HTTP %d", status)
	}
	if _, err := database.Exec(`UPDATE users SET household_id=? WHERE id=?`, user["household_id"], user["user_id"]); err != nil {
		t.Fatal(err)
	}
	var cipher string
	if err := database.QueryRow(`SELECT auth_code_cipher FROM statement_mailbox_configs`).Scan(&cipher); err != nil || cipher == "" || strings.Contains(cipher, firstCode) {
		t.Fatalf("expected ciphertext, query error: %v", err)
	}
	if _, err := client.mailAPI.mailCrypt(cipher, true); err == nil {
		t.Fatal("statement credential accepted under the SMTP encryption purpose")
	}
	if status, _ := store(1, "rejected"); status != http.StatusBadGateway {
		t.Fatalf("failed probe got HTTP %d, want 502", status)
	}
	client.mustOK("POST", path+"/test", map[string]int{"version": 1})
	if tested.Load() != firstCode {
		t.Fatal("failed rotation replaced the saved credential")
	}
	status, config = store(1, rotatedCode)
	if status != http.StatusOK {
		t.Fatalf("rotate got %d: %v", status, config)
	}
	checkConfig(config, 2, true)
	client.mustOK("POST", path+"/test", map[string]int{"version": 2})
	if tested.Load() != rotatedCode {
		t.Fatal("probe did not use the rotated credential")
	}
	client.csrf = ""
	tested.Store("")
	if status, _ := client.do("DELETE", path, map[string]int{"version": 2}); status != http.StatusForbidden {
		t.Fatalf("no-CSRF delete got HTTP %d, want 403", status)
	}
	client.csrf = client.mustOK("GET", "/api/v1/auth/me", nil)["csrf_token"].(string)
	checkConfig(client.mustOK("DELETE", path, map[string]int{"version": 2}), 3, false)
	checkConfig(client.mustOK("GET", path, nil), 3, false)
	if status, _ := store(2, firstCode); status != http.StatusConflict {
		t.Fatalf("stale PUT got HTTP %d, want 409", status)
	}
	if status, _ := client.do("POST", path+"/test", map[string]int{"version": 3}); status != http.StatusConflict || tested.Load() != "" {
		t.Fatal("disabled mailbox was probed")
	}
	var savedEmail string
	if err := database.QueryRow(`SELECT email,auth_code_cipher FROM statement_mailbox_configs`).Scan(&savedEmail, &cipher); err != nil || savedEmail != "" || cipher != "" {
		t.Fatalf("disable should clear saved credentials, query error: %v", err)
	}
	if _, err := database.Exec(`UPDATE users SET role='viewer' WHERE id=?`, user["user_id"]); err != nil {
		t.Fatal(err)
	}
	if status, _ := store(3, firstCode); status != http.StatusForbidden {
		t.Fatalf("viewer PUT got HTTP %d, want 403", status)
	}
}

func TestStatementMailboxSaveRechecksVersionAfterProbe(t *testing.T) {
	const path = "/api/v1/credit-statement-mailbox"
	started := make(chan struct{})
	proceed := make(chan struct{})
	ts, client, database := newTestServerWithOptions(t, Options{
		MailKey: bytes.Repeat([]byte{7}, 32),
		StatementMailboxProbe: func(ctx context.Context, _, code string) error {
			if code == "delayed" {
				close(started)
				select {
				case <-proceed:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	})
	defer ts.Close()
	defer database.Close()
	defer close(proceed)
	client.mustOK("PUT", path, map[string]any{"email": "123@qq.com", "auth_code": "first", "version": 0})

	result := make(chan int, 1)
	go func() {
		status, _ := client.do("PUT", path, map[string]any{"email": "123@qq.com", "auth_code": "delayed", "version": 1})
		result <- status
	}()
	select {
	case <-started:
	case status := <-result:
		t.Fatalf("delayed PUT returned before probe: %d", status)
	case <-time.After(5 * time.Second):
		t.Fatal("delayed PUT did not reach its probe")
	}
	client.mustOK("DELETE", path, map[string]int{"version": 1})
	proceed <- struct{}{}
	if status := <-result; status != http.StatusConflict {
		t.Fatalf("delayed PUT got HTTP %d, want 409", status)
	}
	if current := client.mustOK("GET", path, nil); current["enabled"] != false || current["version"] != float64(2) {
		t.Fatalf("delayed PUT modified disabled config: %v", current)
	}
}

func TestStatementMailboxTestRejectsInvalidInput(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	path := "/api/v1/credit-statement-mailbox/test"
	for _, body := range []map[string]string{
		{"email": "fake@example.com", "auth_code": "dummy"},
		{"email": "123@qq.com", "auth_code": "dummy\r\nA2 SELECT INBOX"},
	} {
		if status, _ := client.do("POST", path, body); status != http.StatusBadRequest {
			t.Fatalf("got HTTP %d, want 400", status)
		}
	}
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request got HTTP %d, want 401", resp.StatusCode)
	}
}
