package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCreditMailConfiguration(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	c.mailAPI.opts.CreditMailKey = bytes.Repeat([]byte{9}, 32)
	c.mailAPI.opts.CreditIMAPProbe = func(_ context.Context, host, user, password, folder string) error {
		if host == "imap.example.com" && user == "test@example.com" && password == "test-authorization-code" && folder == "INBOX" {
			return nil
		}
		return errors.New("probe rejected")
	}
	const base = "/api/v1/credit-mail"
	input := map[string]any{"host": "imap.example.com", "username": "test@example.com", "password": "test-authorization-code", "enabled": true}
	if status, _ := c.do("POST", base+"/mailboxes", map[string]any{"host": "imap.example.com", "username": "test@example.com", "password": "bad", "enabled": true}); status != 502 {
		t.Fatalf("invalid password status %d", status)
	}
	empty := c.mustOK("GET", base, nil)
	if len(empty["mailboxes"].([]any)) != 0 {
		t.Fatal("failed test persisted a mailbox")
	}
	box := c.mustOK("POST", base+"/mailboxes", input)
	id := box["id"].(string)
	path := base + "/mailboxes/" + id
	listed := c.mustOK("GET", base, nil)
	for _, response := range []map[string]any{box, listed} {
		raw, _ := json.Marshal(response)
		if strings.Contains(string(raw), "test-authorization-code") || strings.Contains(string(raw), "password") {
			t.Fatal("API disclosed credentials")
		}
	}
	var stored, household string
	if err := database.QueryRow(`SELECT password_cipher,household_id FROM credit_mailboxes WHERE id=?`, id).Scan(&stored, &household); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "test-authorization-code") {
		t.Fatal("stored password is unencrypted")
	}
	if _, err := creditMailboxCrypt(c.mailAPI.opts.CreditMailKey, household, "another-mailbox", stored, true); err == nil {
		t.Fatal("ciphertext can be replayed across mailboxes")
	}

	delete(input, "password")
	input["version"] = box["version"]
	box = c.mustOK("PUT", path, input)
	if status, _ := c.do("PUT", path, input); status != 409 {
		t.Fatalf("stale update status %d", status)
	}
	input["version"] = box["version"]
	input["host"] = "other.example.com"
	if status, _ := c.do("PUT", path, input); status != 400 {
		t.Fatalf("credential reuse with a different host status %d", status)
	}
	input["host"] = "imap.example.com"

	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "测试浦发卡", "kind": "credit_card", "issuer": "浦发银行", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})
	bindPath := base + "/bindings/" + card["id"].(string)
	if _, err := database.Exec(`INSERT INTO households(id,name) VALUES('foreign-household','隔离测试家庭')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO credit_mailboxes(id,household_id,host,username,folder,password_cipher,enabled,version,tested_at)
		VALUES('foreign-mailbox','foreign-household','imap.example.com','foreign@example.com','INBOX',?,1,1,'')`, stored); err != nil {
		t.Fatal(err)
	}
	if len(c.mustOK("GET", base, nil)["mailboxes"].([]any)) != 1 {
		t.Fatal("foreign mailbox visible")
	}
	if status, _ := c.do("PUT", base+"/mailboxes/foreign-mailbox", input); status != 404 {
		t.Fatalf("foreign mailbox update status %d", status)
	}
	binding := map[string]any{"mailbox_id": "foreign-mailbox", "category_id": c.firstCategory("expense"), "subject_keyword": "信用卡账单", "card_tail": "0123"}
	if status, _ := c.do("PUT", bindPath, binding); status != 400 {
		t.Fatalf("invalid mailbox binding status %d", status)
	}
	binding["mailbox_id"] = id
	c.mustOK("PUT", bindPath, binding)
	if len(c.mustOK("GET", base, nil)["bindings"].([]any)) != 1 {
		t.Fatal("binding missing")
	}

	input["enabled"] = false
	c.mustOK("PUT", path, input)
	if status, _ := c.do("PUT", bindPath, binding); status != 400 {
		t.Fatalf("disabled mailbox binding status %d", status)
	}
	c.mustOK("PUT", bindPath, map[string]string{"mailbox_id": ""})
	if len(c.mustOK("GET", base, nil)["bindings"].([]any)) != 0 {
		t.Fatal("binding was not removed")
	}
}
