package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// Exercise authenticated HTTP reads against a disposable ledger populated
// through the normal posting path. Fixture creation is outside the timers.
func BenchmarkLedgerReads(b *testing.B) {
	const entries = 2500
	ts, c, database := newTestServer(b)
	defer database.Close()
	defer ts.Close()

	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Benchmark checking", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	category := c.firstCategory("expense")
	for i := range entries {
		c.mustOK("POST", "/api/v1/transactions", map[string]any{
			"type": "expense", "amount_minor": 100, "payment_kind": "cash",
			"account_id": account, "occurred_on": fmt.Sprintf("2026-11-%02d", i%30+1),
			"splits": []any{map[string]any{"category_id": category, "amount_minor": 100}},
		})
	}

	const period = "?from=2026-11-01&to=2026-11-30"
	const transactionPath = "/api/v1/transactions" + period
	cursor, ok := c.mustOK("GET", transactionPath, nil)["next_cursor"].(string)
	if !ok || cursor == "" {
		b.Fatal("benchmark fixture did not produce a second page")
	}
	for _, workload := range []struct {
		name, path string
		field      string
		want       int64
	}{
		{"dashboard", "/api/v1/dashboard/summary" + period, "expense_minor", entries * 100},
		{"latest_transactions", transactionPath, "total", 200},
		{"next_transactions", transactionPath + "&cursor=" + cursor, "total", 200},
	} {
		b.Run(workload.name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					resp, err := c.hc.Get(c.base + workload.path)
					if err != nil {
						b.Error(err)
						return
					}
					var body map[string]any
					err = json.NewDecoder(resp.Body).Decode(&body)
					_ = resp.Body.Close()
					if resp.StatusCode != http.StatusOK || err != nil {
						b.Errorf("GET %s: status %d, decode error %v", workload.path, resp.StatusCode, err)
						return
					}
					if number(body[workload.field]) != workload.want {
						b.Errorf("%s = %v, want %d", workload.field, body[workload.field], workload.want)
						return
					}
				}
			})
		})
	}
}
