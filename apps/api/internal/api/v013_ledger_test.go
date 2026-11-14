package api

import (
	"strings"
	"testing"
)

// Fixed numerical acceptance from the handoff plan, plus the boundary cases
// listed in M2. All amounts are integer minor units; no float is used.

func TestV013_ReferenceReturnIsNotAnnualised(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	v012Buy(t, c, asset, broker, "2026-11-01", 100*1_000_000, 100_000, 500, true)
	v012Buy(t, c, asset, broker, "2026-11-02", 100*1_000_000, 120_000, 500, true)
	c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
		"asset_id": asset, "side": "sell", "trade_on": "2026-11-03",
		"quantity_scaled": 50 * 1_000_000, "gross_minor": 65_000,
		"fee_minor": 500, "from_account_id": broker,
	})
	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": "2026-11-10", "market_value_minor": 210_000, "price_scaled": 1400,
	})
	c.mustOK("POST", "/api/v1/investments/dividends", map[string]any{
		"asset_id": asset, "received_on": "2026-11-11", "gross_minor": 10_000,
		"tax_minor": 1_000, "to_account_id": broker,
	})

	pos := v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
	if got := number(pos["total_return_minor"]); got != 62_500 {
		t.Fatalf("total return = %d, want 62500 (625.00)", got)
	}
	// 62500 / 221000 = 28.28% (simple reference return, NOT annualised).
	if got := number(pos["reference_return_bps"]); got != 2_828 {
		t.Errorf("reference return = %d bps, want 2828 (28.28%%)", got)
	}
	summary := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	if got := number(summary["investment_total_return_minor"]); got != 62_500 {
		t.Errorf("dashboard total return = %d, want 62500", got)
	}
	note, _ := summary["investment_valuation_note"].(string)
	if !strings.Contains(note, "不是年化收益率") {
		t.Errorf("dashboard must state the return is NOT annualised: %q", note)
	}
	// Investment principal must never appear as ordinary income/expense.
	if got := number(summary["income_minor"]); got != 0 {
		t.Errorf("investment flows leaked into ordinary income: %d", got)
	}
	if got := number(summary["expense_minor"]); got != 0 {
		t.Errorf("investment flows leaked into ordinary expense: %d", got)
	}
}

func TestV013_InvestmentBoundaryRegression(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	sell := func(on string, qty, gross int64) (int, map[string]any) {
		return c.do("POST", "/api/v1/investments/trades", map[string]any{
			"asset_id": asset, "side": "sell", "trade_on": on,
			"quantity_scaled": qty, "gross_minor": gross,
			"fee_minor": 0, "from_account_id": broker,
		})
	}
	// Selling before any holding must be rejected, not book a negative position.
	if code, body := sell("2026-11-01", 10*1_000_000, 13_000); code == 201 {
		t.Fatalf("sell without holding accepted: %v", body)
	}

	v012Buy(t, c, asset, broker, "2026-11-02", 100*1_000_000, 100_000, 0, true)
	if code, _ := sell("2026-11-03", 120*1_000_000, 156_000); code == 201 {
		t.Error("overselling must be rejected")
	}
	// Sell 40 of 100 at 13.00: net 52000, carried cost 40000, realised 12000.
	if code, body := sell("2026-11-04", 40*1_000_000, 52_000); code != 201 {
		t.Fatalf("first partial sell: %d %v", code, body)
	}
	pos := v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
	if number(pos["quantity_scaled"]) != 60*1_000_000 || number(pos["total_cost_minor"]) != 60_000 ||
		number(pos["realized_pnl_minor"]) != 12_000 {
		t.Errorf("after first partial sell: %v", pos)
	}
	// Clear out the remaining 60 at 13.00: net 78000, carried 60000, realised +18000.
	if code, body := sell("2026-11-05", 60*1_000_000, 78_000); code != 201 {
		t.Fatalf("final sell: %d %v", code, body)
	}
	pos = v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
	if number(pos["quantity_scaled"]) != 0 || number(pos["total_cost_minor"]) != 0 ||
		number(pos["realized_pnl_minor"]) != 30_000 {
		t.Errorf("clear-out must zero cost and keep realised P&L: %v", pos)
	}
	// Re-opening a position must not inherit the previous cycle's cost.
	v012Buy(t, c, asset, broker, "2026-11-06", 50*1_000_000, 50_000, 0, true)
	pos = v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
	if number(pos["quantity_scaled"]) != 50*1_000_000 || number(pos["total_cost_minor"]) != 50_000 ||
		number(pos["realized_pnl_minor"]) != 30_000 {
		t.Errorf("re-opened position polluted by previous cycle: %v", pos)
	}
	// Same-day sell-then-buy is rejected while flat only; after a buy it is fine.
	if code, _ := sell("2026-11-06", 10*1_000_000, 13_000); code != 201 {
		t.Error("same-day sell after re-opening should be allowed")
	}
}

func TestV013_FeeRuleZeroAndMinimumRounding(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	// Zero-rate rule: a trade with no manual override must post fee 0.
	c.mustOK("POST", "/api/v1/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-11-01",
		"commission_bps": 0, "commission_min_minor": 0,
		"stamp_duty_bps": 0, "transfer_fee_bps": 0,
	})
	preview := c.mustOK("POST", "/api/v1/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "buy", "trade_on": "2026-11-10", "gross_minor": 1_000,
	})
	if got := number(preview["total_fee_minor"]); got != 0 {
		t.Errorf("zero-rate preview = %d, want 0", got)
	}
	v012Buy(t, c, asset, broker, "2026-11-10", 10*1_000_000, 1_000, 0, false)
	trades := v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	if got := number(trades[0]["fee_minor"]); got != 0 {
		t.Errorf("zero-rate posted fee = %d, want 0", got)
	}

	// A later version with only a minimum fee: small trades must pay the minimum.
	c.mustOK("POST", "/api/v1/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-12-01",
		"commission_bps": 0, "commission_min_minor": 300,
		"stamp_duty_bps": 0, "transfer_fee_bps": 0,
	})
	preview = c.mustOK("POST", "/api/v1/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "buy", "trade_on": "2026-12-02", "gross_minor": 1_000,
	})
	if got := number(preview["total_fee_minor"]); got != 300 {
		t.Errorf("minimum-fee preview = %d, want 300", got)
	}
	if got := number(preview["fee_rule_version"]); got != 2 {
		t.Errorf("preview must use the effective version 2, got %v", preview["fee_rule_version"])
	}
	v012Buy(t, c, asset, broker, "2026-12-02", 10*1_000_000, 1_000, 0, false)
	trades = v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	found := false
	for _, row := range trades {
		if row["trade_on"] == "2026-12-02" {
			found = true
			if got := number(row["fee_minor"]); got != 300 {
				t.Errorf("minimum-fee posted = %d, want 300", got)
			}
		}
	}
	if !found {
		t.Errorf("minimum-fee trade missing: %v", trades)
	}
}

func TestV013_ValuationWithPastDate(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)
	v012Buy(t, c, asset, broker, "2026-11-14", 100*1_000_000, 100_000, 500, true)

	const old = "2026-09-15"
	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": old, "market_value_minor": 140_000,
	})
	pos := v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))[0]
	if pos["last_valued_on"] != old {
		t.Errorf("last_valued_on = %v, want %s", pos["last_valued_on"], old)
	}
	if got := number(pos["market_value_minor"]); got != 140_000 {
		t.Errorf("market_value_minor = %d, want 140000", got)
	}
	if got := number(pos["unrealized_pnl_minor"]); got != 39_500 {
		t.Errorf("unrealized_pnl_minor = %d, want 39500", got)
	}
	if got := number(pos["reference_return_bps"]); got == 0 {
		t.Errorf("reference_return_bps must be present after valuation: %v", pos)
	}
}
