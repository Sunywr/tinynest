package api

import (
	"sync"
	"testing"
	"time"
)

// V0.1.2 regression suite. Every assertion goes through the real HTTP API and a
// throwaway SQLite database: no mocks, no direct balance writes.
// Written BEFORE the implementation so the missing endpoints are proven by a
// failing test rather than by an assertion in a report.

func v012BrokerAndAsset(t *testing.T, c *testClient) (string, string) {
	t.Helper()
	broker := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "V012 券商账户", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "V012", "name": "V012 测试股票", "kind": "stock", "account_id": broker,
	})["id"].(string)
	return broker, asset
}

func v012Rows(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	items, _ := out["items"].([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("unexpected row type %T", it)
		}
		rows = append(rows, m)
	}
	return rows
}

func v012Find(t *testing.T, rows []map[string]any, key string, want int64) map[string]any {
	t.Helper()
	for _, row := range rows {
		if number(row[key]) == want {
			return row
		}
	}
	t.Fatalf("row %s=%d not found in %v", key, want, rows)
	return nil
}

func v012Buy(t *testing.T, c *testClient, asset, broker, on string, qty, gross, fee int64, withFee bool) {
	t.Helper()
	body := map[string]any{
		"asset_id": asset, "side": "buy", "trade_on": on,
		"quantity_scaled": qty, "gross_minor": gross, "from_account_id": broker,
	}
	if withFee {
		body["fee_minor"] = fee
	}
	c.mustOK("POST", "/api/v1/investments/trades", body)
}

// C8/C9/C12 back end: fee rule listing and preview must exist, be versioned,
// be date-effective, and produce EXACTLY the fee that a real trade posts.
func TestV012_FeeRuleListAndPreview(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	v1 := c.mustOK("POST", "/api/v1/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-11-01",
		"commission_bps": 25, "commission_min_minor": 500,
		"stamp_duty_bps": 10, "transfer_fee_bps": 2,
	})
	if number(v1["version"]) != 1 {
		t.Fatalf("first rule version = %v, want 1", v1["version"])
	}
	v2 := c.mustOK("POST", "/api/v1/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-12-01",
		"commission_bps": 30, "commission_min_minor": 0,
		"stamp_duty_bps": 10, "transfer_fee_bps": 2,
	})
	if number(v2["version"]) != 2 {
		t.Fatalf("second rule version = %v, want 2", v2["version"])
	}

	list := c.mustOK("GET", "/api/v1/investments/fee-rules?account_id="+broker, nil)
	rows := v012Rows(t, list)
	if len(rows) != 2 {
		t.Fatalf("fee rule list = %d rows, want 2: %v", len(rows), list)
	}
	for _, row := range rows {
		for _, key := range []string{
			"id", "account_id", "account_name", "version", "effective_from",
			"commission_bps", "commission_min_minor", "stamp_duty_bps", "transfer_fee_bps",
		} {
			if _, ok := row[key]; !ok {
				t.Errorf("fee rule row missing %q: %v", key, row)
			}
		}
		if row["account_id"] != broker {
			t.Errorf("fee rule row leaks another account: %v", row)
		}
	}

	// v1: commission max(100000*25/10000=250, min 500) = 500; transfer 100000*2/10000=20.
	buy := c.mustOK("POST", "/api/v1/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "buy", "trade_on": "2026-11-15", "gross_minor": 100_000,
	})
	if got := number(buy["commission_minor"]); got != 500 {
		t.Errorf("buy commission = %d, want 500", got)
	}
	if got := number(buy["stamp_duty_minor"]); got != 0 {
		t.Errorf("buy stamp duty = %d, want 0", got)
	}
	if got := number(buy["transfer_fee_minor"]); got != 20 {
		t.Errorf("buy transfer fee = %d, want 20", got)
	}
	if got := number(buy["total_fee_minor"]); got != 520 {
		t.Errorf("buy total fee = %d, want 520", got)
	}
	if buy["fee_rule_id"] != v1["id"] {
		t.Errorf("preview applied %v, want v1 %v", buy["fee_rule_id"], v1["id"])
	}

	sell := c.mustOK("POST", "/api/v1/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "sell", "trade_on": "2026-11-15", "gross_minor": 100_000,
	})
	if got := number(sell["stamp_duty_minor"]); got != 100 {
		t.Errorf("sell stamp duty = %d, want 100", got)
	}
	if got := number(sell["total_fee_minor"]); got != 620 {
		t.Errorf("sell total fee = %d, want 620", got)
	}

	// A later effective_from must win (v2: 300 + 100 + 20 = 420).
	future := c.mustOK("POST", "/api/v1/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "sell", "trade_on": "2026-12-05", "gross_minor": 100_000,
	})
	if got := number(future["total_fee_minor"]); got != 420 {
		t.Errorf("v2 preview total = %d, want 420", got)
	}
	if future["fee_rule_id"] != v2["id"] {
		t.Errorf("future preview applied %v, want v2 %v", future["fee_rule_id"], v2["id"])
	}

	// Real trade (no fee_minor override) must post exactly the previewed amount.
	v012Buy(t, c, asset, broker, "2026-11-15", 100*1_000_000, 100_000, 0, false)
	trades := v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	if len(trades) != 1 {
		t.Fatalf("trades after one buy = %d", len(trades))
	}
	if got := number(trades[0]["fee_minor"]); got != number(buy["total_fee_minor"]) {
		t.Errorf("posted fee %d != previewed fee %d (preview/ledger drift)", got, number(buy["total_fee_minor"]))
	}

	// A single-trade override changes the fee but must not change the rule/snapshot.
	v012Buy(t, c, asset, broker, "2026-11-16", 10*1_000_000, 10_000, 999, true)
	trades = v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	overridden := v012Find(t, trades, "gross_minor", 10_000)
	if got := number(overridden["fee_minor"]); got != 999 {
		t.Errorf("override fee = %d, want 999", got)
	}
	if len(rows) != 2 {
		t.Errorf("override must not create a new fee rule: %v", rows)
	}
}

// C9: invalid preview input must be rejected, never silently produce a fee.
func TestV012_FeePreviewRejectsInvalidInput(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, _ := v012BrokerAndAsset(t, c)

	cases := []map[string]any{
		{"account_id": broker, "side": "buy", "trade_on": "2026-11-15", "gross_minor": 0},
		{"account_id": broker, "side": "buy", "trade_on": "2026-11-15", "gross_minor": -1},
		{"account_id": broker, "side": "hold", "trade_on": "2026-11-15", "gross_minor": 100_000},
		{"account_id": broker, "side": "buy", "trade_on": "not-a-date", "gross_minor": 100_000},
		{"account_id": "acc_foreign", "side": "buy", "trade_on": "2026-11-15", "gross_minor": 100_000},
	}
	for i, body := range cases {
		code, out := c.do("POST", "/api/v1/investments/fee-rules/preview", body)
		// Require a real 400 from an existing endpoint: a 404/405 (missing route)
		// must NOT be able to satisfy this test.
		if code != 400 {
			t.Errorf("case %d: want 400 from fee preview, got %d %v", i, code, out)
		}
	}
}

// C14: fixed numerical acceptance from the handoff prompt, via the real HTTP chain.
// 100 shares x 10.00 + 5.00 fee, then 100 x 12.00 + 5.00 fee => 200 shares, cost 2210.00.
// Sell 50 x 13.00 with 5.00 fee => net 645.00, carried cost 552.50, realised 92.50.
// Remaining 150 shares cost 1657.50; valuation 14.00 => unrealised 442.50.
// Dividend 100.00 gross / 10.00 tax => net 90.00; unknown tax must stay unknown.
func TestV012_InvestmentFixedNumericalAcceptance(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	v012Buy(t, c, asset, broker, "2026-11-01", 100*1_000_000, 100_000, 500, true)
	v012Buy(t, c, asset, broker, "2026-11-02", 100*1_000_000, 120_000, 500, true)
	pos := v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))
	if len(pos) != 1 {
		t.Fatalf("positions = %v", pos)
	}
	if got := number(pos[0]["quantity_scaled"]); got != 200*1_000_000 {
		t.Errorf("quantity after two buys = %d, want 200000000", got)
	}
	if got := number(pos[0]["total_cost_minor"]); got != 221_000 {
		t.Errorf("cost after two buys = %d, want 221000 (2210.00)", got)
	}

	c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
		"asset_id": asset, "side": "sell", "trade_on": "2026-11-03",
		"quantity_scaled": 50 * 1_000_000, "gross_minor": 65_000,
		"fee_minor": 500, "from_account_id": broker,
	})
	trades := v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	sellRow := v012Find(t, trades, "gross_minor", 65_000)
	if got := number(sellRow["net_minor"]); got != 64_500 {
		t.Errorf("sell net = %d, want 64500 (645.00)", got)
	}
	pos = v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))
	if got := number(pos[0]["quantity_scaled"]); got != 150*1_000_000 {
		t.Errorf("quantity after sell = %d, want 150000000", got)
	}
	if got := number(pos[0]["total_cost_minor"]); got != 165_750 {
		t.Errorf("remaining cost = %d, want 165750 (1657.50)", got)
	}
	if got := number(pos[0]["realized_pnl_minor"]); got != 9_250 {
		t.Errorf("realised P&L = %d, want 9250 (92.50)", got)
	}

	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": "2026-11-10", "market_value_minor": 210_000, "price_scaled": 1400,
	})
	c.mustOK("POST", "/api/v1/investments/dividends", map[string]any{
		"asset_id": asset, "received_on": "2026-11-11", "gross_minor": 10_000,
		"tax_minor": 1_000, "to_account_id": broker,
	})
	pos = v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))
	if got := number(pos[0]["net_dividend_minor"]); got != 9_000 {
		t.Errorf("net dividend = %d, want 9000 (90.00)", got)
	}
	if got := number(pos[0]["unrealized_pnl_minor"]); got != 44_250 {
		t.Errorf("unrealised P&L = %d, want 44250 (442.50)", got)
	}
	if got := number(pos[0]["total_return_minor"]); got != 62_500 {
		t.Errorf("total return = %d, want 62500 (92.50+442.50+90.00)", got)
	}
	if pos[0]["last_valued_on"] != "2026-11-10" {
		t.Errorf("valuation date missing: %v", pos[0]["last_valued_on"])
	}
	if pos[0]["dividend_tax_unknown"] != false {
		t.Errorf("known tax must not be reported unknown: %v", pos[0])
	}

	// Unknown tax must NOT be silently converted to zero.
	unknown := c.mustOK("POST", "/api/v1/investments/dividends", map[string]any{
		"asset_id": asset, "received_on": "2026-11-12", "gross_minor": 5_000,
		"to_account_id": broker,
	})
	if unknown["tax_unknown"] != true {
		t.Errorf("unknown dividend tax must be flagged: %v", unknown)
	}
	pos = v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))
	if pos[0]["dividend_tax_unknown"] != true {
		t.Errorf("position must surface unknown dividend tax: %v", pos[0])
	}
}

// C13: net worth breakdown must reconcile with the journal and with the
// existing dashboard summary, and every item must be drill-downable.
func TestV012_NetWorthBreakdownReconciles(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "V012 现金", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	broker := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "V012 券商", "opening_balance_minor": 500_000,
	})["id"].(string)
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "NW1", "name": "V012 资产", "kind": "stock", "account_id": broker,
	})["id"].(string)
	expCat := c.firstCategory("expense")

	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"kind": "credit_card", "name": "V012 信用卡",
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 10, "due_month_offset": 1,
	})["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100_000, "occurred_on": "2026-11-12",
		"payment_kind": "credit", "credit_instrument_id": card,
		"splits": []any{map[string]any{"category_id": expCat, "amount_minor": 100_000}},
	})
	c.mustOK("POST", "/api/v1/debts", map[string]any{
		"kind": "mortgage", "name": "V012 房贷", "principal_minor": 300_000,
		"start_on": "2026-11-01", "due_day": 10, "cycle": "monthly",
	})
	// InstallmentCreate posts on first_due_on (existing behaviour, unchanged).
	// Keep it inside the reporting window so the liability bucket is exercised.
	c.mustOK("POST", "/api/v1/installments", map[string]any{
		"name": "V012 分期", "category_id": expCat,
		"principal_minor": 200_000, "total_fee_minor": 0,
		"periods": 2, "first_due_on": "2026-11-20",
	})
	v012Buy(t, c, asset, broker, "2026-11-13", 100*1_000_000, 100_000, 500, true)
	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": "2026-11-14", "market_value_minor": 120_000, "price_scaled": 1200,
	})

	nw := c.mustOK("GET", "/api/v1/dashboard/net-worth?from=2026-11-01&to=2026-11-30", nil)
	sum := c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	assets, ok := nw["assets"].(map[string]any)
	if !ok {
		t.Fatalf("assets breakdown missing: %v", nw)
	}
	liab, ok := nw["liabilities"].(map[string]any)
	if !ok {
		t.Fatalf("liabilities breakdown missing: %v", nw)
	}
	inv, ok := nw["investment"].(map[string]any)
	if !ok {
		t.Fatalf("investment breakdown missing: %v", nw)
	}
	if got := number(nw["net_worth_minor"]); got != number(sum["net_worth_minor"]) {
		t.Errorf("net worth %d != dashboard summary %d", got, number(sum["net_worth_minor"]))
	}
	if got := number(assets["cash_minor"]); got != 1_000_000 {
		t.Errorf("cash = %d, want 1000000", got)
	}
	if got := number(assets["broker_cash_minor"]); got != 399_500 {
		t.Errorf("broker cash = %d, want 399500", got)
	}
	if got := number(assets["investment_cost_minor"]); got != 100_500 {
		t.Errorf("investment cost = %d, want 100500", got)
	}
	if got := number(assets["total_minor"]); got != 1_500_000 {
		t.Errorf("assets total = %d, want 1500000", got)
	}
	if got := number(assets["cash_minor"]) + number(assets["broker_cash_minor"]) +
		number(assets["investment_cost_minor"]) + number(assets["other_minor"]); got != number(assets["total_minor"]) {
		t.Errorf("asset groups %d != total %d", got, number(assets["total_minor"]))
	}
	if got := number(liab["credit_minor"]); got != 100_000 {
		t.Errorf("credit liability = %d, want 100000", got)
	}
	if got := number(liab["installment_minor"]); got != 200_000 {
		t.Errorf("installment liability = %d, want 200000", got)
	}
	if got := number(liab["loan_minor"]); got != 300_000 {
		t.Errorf("loan liability = %d, want 300000", got)
	}
	if got := number(liab["total_minor"]); got != 600_000 {
		t.Errorf("liabilities total = %d, want 600000", got)
	}
	if got := number(nw["net_worth_minor"]); got != 900_000 {
		t.Errorf("net worth = %d, want 900000", got)
	}
	if got := number(inv["cost_minor"]); got != 100_500 {
		t.Errorf("investment cost = %d, want 100500", got)
	}
	if got := number(inv["market_value_minor"]); got != 120_000 {
		t.Errorf("market value = %d, want 120000", got)
	}
	if got := number(inv["unrealized_minor"]); got != 19_500 {
		t.Errorf("unrealised = %d, want 19500", got)
	}
	if inv["valued_on"] != "2026-11-14" {
		t.Errorf("valuation date = %v, want 2026-11-14", inv["valued_on"])
	}
	if got := number(nw["net_worth_at_market_minor"]); got != 919_500 {
		t.Errorf("net worth at market = %d, want 919500", got)
	}
	if _, ok := nw["drilldown"].(map[string]any); !ok {
		t.Errorf("drilldown hints missing: %v", nw)
	}
	for _, group := range []map[string]any{assets, liab} {
		items, _ := group["items"].([]any)
		if len(items) == 0 {
			t.Errorf("breakdown items empty: %v", group)
		}
		for _, it := range items {
			row, _ := it.(map[string]any)
			if code, _ := row["ledger_code"].(string); code == "" {
				t.Errorf("breakdown item lacks ledger_code: %v", row)
			}
		}
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+cash+"/balance", nil)["balance_minor"]); got != 1_000_000 {
		t.Errorf("cash account balance changed unexpectedly: %d", got)
	}
}

// C15: 20 sequential + 20 concurrent replays of one trade must not double-book.
func TestV012_IdempotentTradeReplay20Times(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()
	broker, asset := v012BrokerAndAsset(t, c)

	body := map[string]any{
		"asset_id": asset, "side": "buy", "trade_on": "2026-11-15",
		"quantity_scaled": 100 * 1_000_000, "gross_minor": 100_000,
		"fee_minor": 500, "from_account_id": broker,
	}
	c.headers = map[string]string{"Idempotency-Key": "v012-once"}
	for i := 0; i < 20; i++ {
		c.do("POST", "/api/v1/investments/trades", body)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.do("POST", "/api/v1/investments/trades", body)
		}()
	}
	wg.Wait()
	c.headers = nil

	trades := v012Rows(t, c.mustOK("GET", "/api/v1/investments/trades", nil))
	if len(trades) != 1 {
		t.Fatalf("40 replays produced %d trades, want 1", len(trades))
	}
	pos := v012Rows(t, c.mustOK("GET", "/api/v1/investments/positions", nil))
	if got := number(pos[0]["quantity_scaled"]); got != 100*1_000_000 {
		t.Errorf("quantity after replays = %d, want 100000000", got)
	}
	if got := number(c.mustOK("GET", "/api/v1/accounts/"+broker+"/balance", nil)["balance_minor"]); got != 899_500 {
		t.Errorf("cash double-booked: balance = %d, want 899500", got)
	}
}

// C15: the new endpoints must never see another household's data.
func TestV012_NewEndpointsAreHouseholdScoped(t *testing.T) {
	now := time.Date(2026, 11, 14, 4, 0, 0, 0, time.UTC)
	e := newRemEnv(t, Options{Clock: func() time.Time { return now }, SetupToken: testSetupToken})
	defer e.ts.Close()
	defer e.database.Close()
	ha := bootstrapHousehold(t, e, "v12a")
	hb := bootstrapHousehold(t, e, "v12b")
	ca := ha.client(t, e.ts.URL)
	cb := hb.client(t, e.ts.URL)
	call := func(c *remClient, method, path string, body any) remResult {
		return c.call(method, "/api/v1"+path, body, nil)
	}

	broker := call(ca, "POST", "/accounts", map[string]any{
		"kind": "securities", "name": "A 券商", "opening_balance_minor": 1_000_000,
	}).Body["id"].(string)
	rule := call(ca, "POST", "/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-11-01",
		"commission_bps": 25, "commission_min_minor": 500,
		"stamp_duty_bps": 10, "transfer_fee_bps": 2,
	})
	if rule.Status != 201 {
		t.Fatalf("A create fee rule: %d %v", rule.Status, rule.Body)
	}
	asset := call(ca, "POST", "/investments/assets", map[string]any{
		"code": "A1", "name": "A 股票", "kind": "stock", "account_id": broker,
	}).Body["id"].(string)
	trade := call(ca, "POST", "/investments/trades", map[string]any{
		"asset_id": asset, "side": "buy", "trade_on": "2026-11-14",
		"quantity_scaled": 10 * 1_000_000, "gross_minor": 10_000,
		"fee_minor": 500, "from_account_id": broker,
	})
	if trade.Status != 201 {
		t.Fatalf("A trade: %d %v", trade.Status, trade.Body)
	}

	listA := call(ca, "GET", "/investments/fee-rules", nil)
	if got := len(v012Rows(t, listA.Body)); got != 1 {
		t.Errorf("A must see its own single fee rule, got %d", got)
	}
	for _, query := range []string{"/investments/fee-rules", "/investments/fee-rules?account_id=" + broker} {
		listB := call(cb, "GET", query, nil)
		if listB.Status != 200 {
			t.Fatalf("B list %s: %d %v", query, listB.Status, listB.Body)
		}
		if got := len(v012Rows(t, listB.Body)); got != 0 {
			t.Errorf("B saw %d foreign fee rules via %s", got, query)
		}
	}
	preview := call(cb, "POST", "/investments/fee-rules/preview", map[string]any{
		"account_id": broker, "side": "buy", "trade_on": "2026-11-14", "gross_minor": 100_000,
	})
	if preview.Status >= 200 && preview.Status < 300 {
		t.Errorf("B previewed A's fee rule: %d %v", preview.Status, preview.Body)
	}
	createForeign := call(cb, "POST", "/investments/fee-rules", map[string]any{
		"account_id": broker, "effective_from": "2026-11-01", "commission_bps": 99,
	})
	if createForeign.Status == 201 {
		t.Errorf("B attached a fee rule to A's account: %v", createForeign.Body)
	}

	to := "?from=2026-11-01&to=2026-11-30"
	nwB := call(cb, "GET", "/dashboard/net-worth"+to, nil)
	if nwB.Status != 200 {
		t.Fatalf("B net worth: %d %v", nwB.Status, nwB.Body)
	}
	bAssets, _ := nwB.Body["assets"].(map[string]any)
	if got := number(bAssets["total_minor"]); got != 0 {
		t.Errorf("B assets include A's ledger: %d", got)
	}
	nwA := call(ca, "GET", "/dashboard/net-worth"+to, nil)
	aAssets, _ := nwA.Body["assets"].(map[string]any)
	if got := number(aAssets["total_minor"]); got != 1_000_000 {
		t.Errorf("A assets = %d, want 1000000 (buy is an asset conversion)", got)
	}
}
