package api

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"
)

// TestMixedLedgerLoad reports a bounded, synthetic HTTP workload. The timing
// samples are observations; the test asserts successful requests and balances.
func TestMixedLedgerLoad(t *testing.T) {
	const workers, rounds = 8, 32
	const openingBalance, debit = 100_000, 100
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	account := client.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "Mixed workload", "opening_balance_minor": openingBalance,
	})["id"].(string)
	category := client.firstCategory("expense")
	payload := map[string]any{
		"type": "expense", "amount_minor": debit, "occurred_on": "2026-11-12",
		"payment_kind": "cash", "account_id": account,
		"splits": []any{map[string]any{"category_id": category, "amount_minor": debit}},
	}
	balancePath := "/api/v1/accounts/" + account + "/balance"
	expectedBalance := int64(openingBalance - workers*rounds*debit)
	reads := make([]time.Duration, workers*rounds)
	writes := make([]time.Duration, len(reads))
	start := time.Now()
	passed := t.Run("mixed requests", func(t *testing.T) {
		for worker := range workers {
			t.Run(fmt.Sprint(worker), func(t *testing.T) {
				t.Parallel()
				requester := *client
				requester.t = t
				for round := range rounds {
					sample := worker*rounds + round
					before := time.Now()
					status, out := requester.do("POST", "/api/v1/transactions", payload)
					writes[sample] = time.Since(before)
					if status != http.StatusCreated || out["id"] == nil {
						t.Fatalf("POST transaction: %d %v", status, out)
					}
					before = time.Now()
					status, out = requester.do("GET", balancePath, nil)
					reads[sample] = time.Since(before)
					balance := number(out["balance_minor"])
					if status != http.StatusOK || balance < expectedBalance || balance > openingBalance || balance%debit != 0 {
						t.Fatalf("GET balance: %d %v", status, out)
					}
				}
			})
		}
	})
	elapsed := time.Since(start)
	if !passed {
		return
	}
	if got := number(client.mustOK("GET", balancePath, nil)["balance_minor"]); got != expectedBalance {
		t.Fatalf("final balance: got %d, want %d", got, expectedBalance)
	}
	t.Logf("%d clients, %d requests in %v, %.1f requests/s", workers, len(reads)+len(writes), elapsed, float64(len(reads)+len(writes))/elapsed.Seconds())
	logLedgerLatencies(t, "read", reads)
	logLedgerLatencies(t, "write", writes)
}

func logLedgerLatencies(t *testing.T, operation string, timings []time.Duration) {
	t.Helper()
	slices.Sort(timings)
	nearestRank := func(percent int) time.Duration {
		return timings[(len(timings)*percent+99)/100-1]
	}
	t.Logf("%s latency p50=%v p95=%v p99=%v", operation, nearestRank(50), nearestRank(95), nearestRank(99))
}
