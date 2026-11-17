package api

import (
	"database/sql"
	"testing"
)

// Early-trade corrections: reversing a buy or sell that is NOT the asset's last
// posted trade replays the remaining trades in posting order, restates each
// later sell's moving weighted cost and realised P&L, and posts one balancing
// correction entry per restated sell so the journal keeps agreeing with the
// rebuilt position. All numbers below are exact integer minor units.

func tradeFixture(t *testing.T, c *testClient) (string, string) {
	t.Helper()
	broker := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "更正测试券商", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "FIX", "name": "更正测试股票", "kind": "stock", "account_id": broker,
	})["id"].(string)
	return broker, asset
}

func firstPosition(t *testing.T, c *testClient) map[string]any {
	t.Helper()
	return v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
}

func correctionCount(t *testing.T, database *sql.DB, tradeID string) int64 {
	t.Helper()
	var n int64
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM journal_entries WHERE biz_type='invest_cost_correction' AND biz_id=?`, tradeID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReverseEarlyTradeRecalculatesLaterSells(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := tradeFixture(t, c)
	const base = "/api/v1/investments/trades"
	trade := func(side, on string, qty, gross, fee int64) string {
		return c.mustOK("POST", base, map[string]any{
			"asset_id": asset, "side": side, "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross, "fee_minor": fee,
			"from_account_id": broker,
		})["id"].(string)
	}
	b1 := trade("buy", "2026-11-01", 100_000_000, 100_000, 500)
	trade("buy", "2026-11-02", 100_000_000, 120_000, 500)
	s1 := trade("sell", "2026-11-03", 50_000_000, 65_000, 500)
	trade("buy", "2026-11-04", 50_000_000, 50_000, 0)

	pos := firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 200_000_000 || number(pos["total_cost_minor"]) != 215_750 ||
		number(pos["realized_pnl_minor"]) != 9_250 {
		t.Fatalf("baseline position: %v", pos)
	}

	reason := map[string]string{"reason": "首笔买入误录"}
	first := c.mustOK("POST", base+"/"+b1+"/reverse", reason)
	if first["reversal_entry_id"] == nil {
		t.Fatalf("missing reversal entry id: %v", first)
	}
	replay := c.mustOK("POST", base+"/"+b1+"/reverse", reason)
	if replay["reversal_entry_id"] != first["reversal_entry_id"] {
		t.Fatalf("early reversal not idempotent: %v / %v", first, replay)
	}

	// Replayed: b2 cost 120_500; s1 carry 60_250, realised 4_250; b3 -> qty 100, cost 110_250.
	pos = firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 100_000_000 || number(pos["total_cost_minor"]) != 110_250 ||
		number(pos["realized_pnl_minor"]) != 4_250 || number(pos["avg_cost_minor"]) != 1_103 {
		t.Fatalf("replayed position: %v", pos)
	}

	// Cash only regains the reversed buy; corrections never touch cash accounts.
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+broker+"/balance", nil)["balance_minor"]); got != 894_000 {
		t.Fatalf("broker balance = %d, want 894000", got)
	}

	// The journal must keep agreeing with the rebuilt projection.
	nw := c.mustOK("GET", "/api/v1/dashboard/net-worth", nil)
	assets := nw["assets"].(map[string]any)
	if got := number(assets["investment_cost_minor"]); got != 110_250 {
		t.Fatalf("journal investment cost = %d, want 110250", got)
	}
	inv := nw["investment"].(map[string]any)
	if got := number(inv["cost_minor"]); got != 110_250 {
		t.Fatalf("position cost = %d, want 110250", got)
	}
	summary := c.mustOK("GET", "/api/v1/dashboard/summary", nil)
	if got := number(summary["investment_realized_minor"]); got != 4_250 {
		t.Fatalf("dashboard realised = %d, want 4250", got)
	}

	// Trade list: b1 reversed without a reverse button; later trades stay reversible
	// and carry their restated per-trade realised P&L.
	rows := v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades?order=posted", nil))
	byID := map[string]map[string]any{}
	for _, r := range rows {
		byID[r["id"].(string)] = r
	}
	if byID[b1]["status"] != "reversed" || byID[b1]["can_reverse"] != false {
		t.Fatalf("reversed trade row: %v", byID[b1])
	}
	if byID[s1]["can_reverse"] != true || number(byID[s1]["realized_pnl_minor"]) != 4_250 {
		t.Fatalf("restated sell row: %v", byID[s1])
	}

	// Exactly one correction entry, chained to the restated sell and backdated to
	// the sell's own business date; the idempotent replay must not duplicate it.
	if n := correctionCount(t, database.DB, s1); n != 1 {
		t.Fatalf("correction entries for restated sell = %d, want 1", n)
	}
	var corrOccurred string
	if err := database.QueryRow(
		`SELECT occurred_on FROM journal_entries WHERE biz_type='invest_cost_correction' AND biz_id=?`, s1).
		Scan(&corrOccurred); err != nil {
		t.Fatal(err)
	}
	if corrOccurred != "2026-11-03" {
		t.Fatalf("correction occurred_on = %s, want the sell date 2026-11-03", corrOccurred)
	}

	logs := v012Rows(t, c.mustOK("GET", "/api/v1/admin/audit-logs?limit=100", nil))
	found := false
	for _, l := range logs {
		if l["action"] == "reverse_investment_trade" && l["entity_id"] == b1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit log missing reverse_investment_trade for %s", b1)
	}
}

func TestReverseEarlyTradeBlockedByLaterOversell(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := tradeFixture(t, c)
	const base = "/api/v1/investments/trades"
	trade := func(side, on string, qty, gross int64) string {
		return c.mustOK("POST", base, map[string]any{
			"asset_id": asset, "side": side, "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross, "fee_minor": 0,
			"from_account_id": broker,
		})["id"].(string)
	}
	b1 := trade("buy", "2026-11-01", 100_000_000, 100_000)
	s1 := trade("sell", "2026-11-02", 100_000_000, 110_000)
	b2 := trade("buy", "2026-11-03", 100_000_000, 100_000)

	reason := map[string]string{"reason": "首笔买入录错"}
	if status, body := c.do("POST", base+"/"+b1+"/reverse", reason); status != 409 {
		t.Fatalf("overselling replay must be rejected: %d %v", status, body)
	}
	pos := firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 100_000_000 || number(pos["total_cost_minor"]) != 100_000 ||
		number(pos["realized_pnl_minor"]) != 10_000 {
		t.Fatalf("rejected reversal changed the position: %v", pos)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+broker+"/balance", nil)["balance_minor"]); got != 910_000 {
		t.Fatalf("rejected reversal moved cash: %d", got)
	}
	if n := correctionCount(t, database.DB, s1); n != 0 {
		t.Fatalf("rejected reversal posted corrections: %d", n)
	}

	// Unwind from the newest trade backwards; every step now succeeds.
	c.mustOK("POST", base+"/"+b2+"/reverse", reason)
	c.mustOK("POST", base+"/"+s1+"/reverse", reason)
	c.mustOK("POST", base+"/"+b1+"/reverse", reason)
	pos = firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 0 || number(pos["total_cost_minor"]) != 0 ||
		number(pos["realized_pnl_minor"]) != 0 {
		t.Fatalf("final position after full unwind: %v", pos)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+broker+"/balance", nil)["balance_minor"]); got != 1_000_000 {
		t.Fatalf("cash not fully restored: %d", got)
	}
}

func TestReverseEarlyTradeRestatesLossToZero(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := tradeFixture(t, c)
	const base = "/api/v1/investments/trades"
	trade := func(side, on string, qty, gross int64) string {
		return c.mustOK("POST", base, map[string]any{
			"asset_id": asset, "side": side, "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross, "fee_minor": 0,
			"from_account_id": broker,
		})["id"].(string)
	}
	trade("buy", "2026-11-01", 100_000_000, 100_000)
	b2 := trade("buy", "2026-11-02", 100_000_000, 150_000)
	s1 := trade("sell", "2026-11-03", 100_000_000, 100_000)
	trade("buy", "2026-11-04", 100_000_000, 100_000)

	if status, body := c.do("POST", base+"/"+b2+"/reverse", map[string]string{"reason": "第二笔买入误录"}); status != 200 {
		t.Fatalf("reverse middle buy: %d %v", status, body)
	}

	// Replayed: b1 cost 100_000; s1 carries the full 100_000 and breaks even; b3 -> cost 100_000.
	pos := firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 100_000_000 || number(pos["total_cost_minor"]) != 100_000 ||
		number(pos["realized_pnl_minor"]) != 0 {
		t.Fatalf("replayed position: %v", pos)
	}
	nw := c.mustOK("GET", "/api/v1/dashboard/net-worth", nil)
	if got := number(nw["assets"].(map[string]any)["investment_cost_minor"]); got != 100_000 {
		t.Fatalf("journal investment cost = %d, want 100000", got)
	}
	// The old 25_000 realised loss must be fully restated away by the correction chain.
	if n := correctionCount(t, database.DB, s1); n != 1 {
		t.Fatalf("correction entries for restated sell = %d, want 1", n)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+broker+"/balance", nil)["balance_minor"]); got != 900_000 {
		t.Fatalf("broker balance = %d, want 900000", got)
	}
}

func TestReverseTradeRequiresUnambiguousOrder(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := tradeFixture(t, c)
	const base = "/api/v1/investments/trades"
	trade := func(side, on string, qty, gross int64) string {
		return c.mustOK("POST", base, map[string]any{
			"asset_id": asset, "side": side, "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross, "fee_minor": 0,
			"from_account_id": broker,
		})["id"].(string)
	}
	b1 := trade("buy", "2026-11-01", 100_000_000, 100_000)
	b2 := trade("buy", "2026-11-02", 100_000_000, 120_000)

	// Simulate legacy rows whose clock tick cannot establish an order.
	if _, err := database.Exec(
		`UPDATE investment_trades SET created_at='2026-11-01T00:00:00.000Z' WHERE id IN (?,?)`, b1, b2); err != nil {
		t.Fatal(err)
	}
	reason := map[string]string{"reason": "买入误录"}
	if status, body := c.do("POST", base+"/"+b1+"/reverse", reason); status != 409 {
		t.Fatalf("ambiguous order must stay rejected: %d %v", status, body)
	}
	if status, _ := c.do("POST", base+"/"+b2+"/reverse", reason); status != 409 {
		t.Fatalf("ambiguous order must stay rejected for both trades: %d", status)
	}
	pos := firstPosition(t, c)
	if number(pos["quantity_scaled"]) != 200_000_000 || number(pos["total_cost_minor"]) != 220_000 {
		t.Fatalf("rejected reversal changed the position: %v", pos)
	}
}
