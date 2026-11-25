package api

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestStatementCardBindingLifecycle(t *testing.T) {
	server, client, database := newTestServerWithOptions(t, Options{
		MailKey:               bytes.Repeat([]byte{7}, 32),
		StatementMailboxProbe: func(context.Context, string, string) error { return nil },
	})
	defer database.Close()
	defer server.Close()

	const mailbox = "/api/v1/credit-statement-mailbox"
	card := client.mustOK("POST", "/api/v1/accounts", map[string]string{"kind": "credit", "name": "测试信用卡"})
	cardPath := "/api/v1/accounts/" + card["id"].(string)
	binding := cardPath + "/credit-statement-binding"
	assertStatus := func(method, path string, body any, want int) {
		t.Helper()
		if status, result := client.do(method, path, body); status != want {
			t.Fatalf("%s %s: got %d %v, want %d", method, path, status, result, want)
		}
	}
	save := func(email string, version int) {
		t.Helper()
		client.mustOK("PUT", mailbox, map[string]any{"email": email, "auth_code": "fixture", "version": version})
	}
	isBound := func() bool {
		t.Helper()
		items := client.mustOK("GET", mailbox+"/cards", nil)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected one eligible card, got %v", items)
		}
		return items[0].(map[string]any)["bound"] == true
	}
	assertStatus("PUT", binding+"?version=1", nil, http.StatusConflict)
	save("123@qq.com", 0)
	assertStatus("PUT", binding+"?version=1", nil, http.StatusNoContent)
	assertStatus("PUT", binding+"?version=1", nil, http.StatusNoContent)
	if !isBound() {
		t.Fatal("binding missing after save")
	}
	otherCard := client.mustOK("POST", "/api/v1/accounts", map[string]string{"kind": "credit", "name": "163 信用卡"})
	otherPath := "/api/v1/accounts/" + otherCard["id"].(string)
	client.mustOK("PUT", mailbox+"?provider=163", map[string]any{"email": "fixture@163.com", "auth_code": "fixture", "version": 0})
	client.mustOK("POST", mailbox+"/test?provider=163", map[string]int{"version": 1})
	assertStatus("PUT", binding+"?version=1&provider=163", nil, http.StatusConflict)
	assertStatus("PUT", otherPath+"/credit-statement-binding?version=1&provider=163", nil, http.StatusNoContent)
	if current := client.mustOK("GET", mailbox+"?provider=163", nil); current["provider"] != "163" || current["masked_email"] != "***@163.com" {
		t.Fatalf("unexpected 163 configuration: %v", current)
	}
	client.mustOK("DELETE", mailbox+"?provider=163", map[string]int{"version": 1})
	client.mustOK("DELETE", otherPath, nil)
	if !isBound() {
		t.Fatal("disabling 163 removed a QQ binding")
	}
	me := client.mustOK("GET", "/api/v1/auth/me", nil)
	if _, err := database.Exec(`INSERT INTO users(id,household_id,username,password_hash,role)
		VALUES('binding_other',?,'binding_other','fixture','member')`, me["household_id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE sessions SET user_id='binding_other' WHERE user_id=?`, me["user_id"]); err != nil {
		t.Fatal(err)
	}
	save("789@qq.com", 0)
	assertStatus("PUT", binding+"?version=1", nil, http.StatusConflict)
	assertStatus("DELETE", binding+"?version=1", nil, http.StatusNotFound)
	if _, err := database.Exec(`INSERT INTO households(id,name) VALUES('hh_binding_other','其他家庭')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE users SET household_id='hh_binding_other' WHERE id='binding_other'`); err != nil {
		t.Fatal(err)
	}
	save("789@qq.com", 0)
	assertStatus("PUT", binding+"?version=1", nil, http.StatusNotFound)
	if items := client.mustOK("GET", mailbox+"/cards", nil)["items"].([]any); len(items) != 0 {
		t.Fatalf("another household can see cards: %v", items)
	}
	if _, err := database.Exec(`UPDATE sessions SET user_id=? WHERE user_id='binding_other'`, me["user_id"]); err != nil {
		t.Fatal(err)
	}
	save("123@qq.com", 1)
	if !isBound() {
		t.Fatal("rotating the same mailbox should retain the binding")
	}
	assertStatus("DELETE", binding+"?version=1", nil, http.StatusConflict)
	client.mustOK("PATCH", cardPath, map[string]string{"status": "disabled"})
	assertStatus("PUT", binding+"?version=2", nil, http.StatusBadRequest)
	assertStatus("DELETE", binding+"?version=2", nil, http.StatusNoContent)
	client.mustOK("PATCH", cardPath, map[string]string{"status": "active"})
	assertStatus("PUT", binding+"?version=2", nil, http.StatusNoContent)
	save("456@qq.com", 2)
	if isBound() {
		t.Fatal("replacing the mailbox address must clear bindings")
	}
	assertStatus("PUT", binding+"?version=3", nil, http.StatusNoContent)
	client.mustOK("DELETE", mailbox, map[string]int{"version": 3})
	if isBound() {
		t.Fatal("disabling the mailbox must release bindings")
	}
	save("456@qq.com", 4)
	assertStatus("PUT", binding+"?version=5", nil, http.StatusNoContent)
	client.mustOK("DELETE", cardPath, nil)
	assertStatus("PUT", binding+"?version=5", nil, http.StatusNotFound)
	var bindings, entries int
	if err := database.QueryRow(`SELECT COUNT(*) FROM statement_mailbox_card_links`).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatalf("deleted card retained a binding: %d, %v", bindings, err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("binding produced ledger entries: %d, %v", entries, err)
	}
}
