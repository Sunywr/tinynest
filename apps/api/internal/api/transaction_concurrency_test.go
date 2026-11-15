package api

import (
	"fmt"
	"net/http"
	"testing"
)

func TestConcurrentTransactionReplays(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	account := client.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Concurrent checking", "opening_balance_minor": 1000,
	})["id"].(string)
	category := client.firstCategory("expense")
	const transactionPath = "/api/v1/transactions"
	payload := map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-12",
		"payment_kind": "cash", "account_id": account,
		"splits": []any{map[string]any{"category_id": category, "amount_minor": 100}},
	}
	const workers = 16
	type result struct {
		code int
		id   any
	}
	results := make(chan result, workers)
	t.Run("simultaneous submissions", func(t *testing.T) {
		for i := range workers {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				requester := *client
				requester.t = t
				requester.headers = map[string]string{"Idempotency-Key": "concurrent-expense"}
				code, body := requester.do("POST", transactionPath, payload)
				results <- result{code, body["id"]}
			})
		}
	})
	close(results)
	created, replayed := 0, 0
	var id any
	for response := range results {
		switch response.code {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			replayed++
		default:
			t.Errorf("submission returned %d", response.code)
		}
		if id == nil {
			id = response.id
		}
		if response.id == nil || id != response.id {
			t.Errorf("transaction id = %v, want %v", response.id, id)
		}
	}
	if created != 1 || replayed != workers-1 {
		t.Errorf("created %d, replayed %d", created, replayed)
	}
	balance := client.mustOK("GET", "/api/v1/accounts/"+account+"/balance", nil)
	if number(balance["balance_minor"]) != 900 {
		t.Errorf("expected one debit from the account, got balance %v", balance)
	}
}
