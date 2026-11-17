package api

import (
	"database/sql"
	"net/http/httptest"
	"testing"
)

func TestReplaceTransaction(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Correction account", "opening_balance_minor": 1000,
	})["id"].(string)
	category := c.firstCategory("expense")
	const list = "/api/v1/transactions"
	body := func(amount int) map[string]any {
		return map[string]any{
			"type": "expense", "amount_minor": amount, "occurred_on": "2026-11-12",
			"payment_kind": "cash", "account_id": account,
			"splits": []map[string]any{{"category_id": category, "amount_minor": amount}},
		}
	}
	original := list + "/" + c.mustOK("POST", list, body(100))["id"].(string)
	invalid := body(200)
	invalid["account_id"] = "missing"
	if code, _ := c.do("POST", original+"/replace", invalid); code != 400 {
		t.Errorf("invalid replacement returned %d", code)
	}
	if got := c.mustOK("GET", original, nil)["status"]; got != "posted" {
		t.Fatalf("failed replacement changed original status to %v", got)
	}
	c.headers = map[string]string{"Idempotency-Key": "replace-once"}
	replacement := c.mustOK("POST", original+"/replace", body(250))
	replay := c.mustOK("POST", original+"/replace", body(250))
	c.headers = nil
	if replacement["id"] != replay["id"] {
		t.Fatalf("replacement replay returned %v, want %v", replay, replacement)
	}
	if got := c.mustOK("GET", original, nil)["status"]; got != "reversed" {
		t.Fatalf("original status = %v", got)
	}
	assertBalance := func(want int64) {
		t.Helper()
		balance := c.mustOK("GET", "/api/v1/accounts/"+account+"/balance", nil)
		if number(balance["balance_minor"]) != want {
			t.Fatalf("balance = %v, want %d", balance, want)
		}
	}
	assertBalance(750)
	detail := c.mustOK("GET", list+"/"+replacement["id"].(string), nil)
	splits, ok := detail["splits"].([]any)
	if !ok || len(splits) != 1 || detail["account_id"] != account {
		t.Fatalf("replacement detail cannot prefill a new entry: %v", detail)
	}
	if code, _ := c.do("POST", list+"/"+replacement["id"].(string)+"/reverse", nil); code != 404 {
		t.Fatalf("retired endpoint returned %d", code)
	}
	assertBalance(750)
}

func TestCreditTransactionCorrection(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "Correction card", "cycle_start_day": 1,
		"cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"]
	category := c.firstCategory("expense")
	const list = "/api/v1/transactions"
	body := func(amount int, date string) map[string]any {
		return map[string]any{
			"type": "expense", "amount_minor": amount, "occurred_on": date,
			"payment_kind": "credit", "credit_instrument_id": card,
			"splits": []map[string]any{{"category_id": category, "amount_minor": amount}},
		}
	}
	original := list + "/" + c.mustOK("POST", list, body(100, "2026-10-12"))["id"].(string)
	replacement := c.mustOK("POST", original+"/replace", body(75, "2026-11-12"))["id"].(string)
	assertDue := func(cycle string, want int64) {
		t.Helper()
		var got int64
		if err := database.QueryRow("SELECT system_due_minor FROM credit_statements WHERE instrument_id=? AND cycle_start_on=?",
			card, cycle).Scan(&got); err != nil || got != want {
			t.Fatalf("cycle %s: due = %d, want %d, error = %v", cycle, got, want, err)
		}
	}
	assertDue("2026-10-01", 0)
	assertDue("2026-11-01", 75)
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Correction payments", "opening_balance_minor": 1000,
	})["id"]
	var stmtID string
	if err := database.QueryRow("SELECT id FROM credit_statements WHERE instrument_id=? AND cycle_start_on=?", card, "2026-11-01").Scan(&stmtID); err != nil {
		t.Fatal(err)
	}
	c.mustOK("POST", "/api/v1/credit-statements/"+stmtID+"/payments", map[string]any{
		"amount_minor": 25, "paid_on": "2026-11-15", "from_account_id": account,
	})
	if code, _ := c.do("POST", list+"/"+replacement+"/replace", body(50, "2026-11-12")); code != 400 {
		t.Fatalf("correcting a transaction with a paid statement returned %d", code)
	}
	assertDue("2026-11-01", 75)
	if got := c.mustOK("GET", list+"/"+replacement, nil)["status"]; got != "posted" {
		t.Fatalf("denied reversal changed status to %v", got)
	}
}

func TestRestoreStandaloneTransactionReversals(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	const endpoint = "/api/v1/transactions"
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Legacy cash", "opening_balance_minor": 1000,
	})["id"]
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "Legacy card", "cycle_start_day": 1,
		"cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"]
	category := c.firstCategory("expense")
	body := func(amount int) map[string]any {
		return map[string]any{
			"type": "expense", "amount_minor": amount, "occurred_on": "2026-11-12",
			"payment_kind": "cash", "account_id": account,
			"splits": []map[string]any{{"category_id": category, "amount_minor": amount}},
		}
	}
	cashID := c.mustOK("POST", endpoint, body(100))["id"].(string)
	credit := body(300)
	delete(credit, "account_id")
	credit["payment_kind"], credit["credit_instrument_id"] = "credit", card
	creditID := c.mustOK("POST", endpoint, credit)["id"].(string)
	corrected := c.mustOK("POST", endpoint, body(50))["id"].(string)
	c.mustOK("POST", endpoint+"/"+corrected+"/replace", body(75))

	var principal Principal
	principal.Username = "admin"
	if err := database.QueryRow("SELECT household_id FROM transactions WHERE id=?", cashID).Scan(&principal.HouseholdID); err != nil {
		t.Fatal(err)
	}
	a := New(database, Options{})
	r := httptest.NewRequest("POST", endpoint, nil)
	for _, id := range []string{cashID, creditID} {
		if err := database.WithTx(r.Context(), func(tx *sql.Tx) error {
			return a.reverseTransaction(tx, r, principal, id, newID("je"))
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec("DELETE FROM schema_migrations WHERE version=(SELECT MAX(version) FROM (SELECT version FROM schema_migrations) v)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(r.Context()); err != nil {
		t.Fatal(err)
	}
	if applied, err := database.Migrate(r.Context()); err != nil || len(applied) != 0 {
		t.Fatalf("second migration = %v, %v", applied, err)
	}
	for _, id := range []string{cashID, creditID} {
		if got := c.mustOK("GET", endpoint+"/"+id, nil)["status"]; got != "posted" {
			t.Fatalf("legacy transaction %s status = %v", id, got)
		}
	}
	if got := c.mustOK("GET", endpoint+"/"+corrected, nil)["status"]; got != "reversed" {
		t.Fatalf("correction was changed: %v", got)
	}
	balance := c.mustOK("GET", "/api/v1/accounts/"+account.(string)+"/balance", nil)
	if got := number(balance["balance_minor"]); got != 825 {
		t.Fatalf("restored balance = %d, want 825", got)
	}
	var due int64
	if err := database.QueryRow("SELECT system_due_minor FROM credit_statements WHERE instrument_id=?", card).Scan(&due); err != nil || due != 300 {
		t.Fatalf("restored bill = %d, want 300, error = %v", due, err)
	}
}
