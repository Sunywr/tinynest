package api

import (
	"context"
	"testing"
)

func TestInvestmentOverviewTotalsAndIsolation(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	ctx := context.Background()

	var householdID string
	if err := database.QueryRowContext(ctx, `SELECT id FROM households LIMIT 1`).Scan(&householdID); err != nil {
		t.Fatal(err)
	}
	memberID := newID("mem")
	if _, err := database.ExecContext(ctx,
		`INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`, memberID, householdID, "爸爸"); err != nil {
		t.Fatal(err)
	}

	secID := newID("inv")
	wealthID := newID("inv")
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_accounts
		(id,household_id,kind,name,institution,owner_member_id,cash_minor,dividends_minor)
		VALUES(?,?,?,?,?,?,?,?)`,
		secID, householdID, "security", "证券账户", "测试证券", memberID, 500000, 10000); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_accounts
		(id,household_id,kind,name,institution,owner_member_id,cash_minor,dividends_minor)
		VALUES(?,?,?,?,?,?,?,?)`,
		wealthID, householdID, "wealth", "理财账户", "", memberID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_holdings
		(id,household_id,account_id,symbol,name,asset_class,asset_label,quantity_micros,price_micros,
		 cost_minor,market_value_minor,unrealized_pnl_minor,dividends_minor,last_trade_on,
		 last_trade_pnl_minor,last_trade_return_bp)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		newID("hold"), householdID, secID, "600900", "长江电力", "stock", "股票", 1000000, 120000,
		100000, 120000, 20000, 5000, "2026-11-01", 1000, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_holdings
		(id,household_id,account_id,symbol,name,asset_class,asset_label,quantity_micros,price_micros,
		 cost_minor,market_value_minor,unrealized_pnl_minor,dividends_minor)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		newID("hold"), householdID, wealthID, "133042A", "招银理财", "wealth", "理财", 0, 0,
		50000, 40000, -10000, 0); err != nil {
		t.Fatal(err)
	}

	// A different household must never leak into the projection.
	otherHousehold := newID("hh")
	if _, err := database.ExecContext(ctx, `INSERT INTO households(id,name) VALUES(?,?)`, otherHousehold, "其他家庭"); err != nil {
		t.Fatal(err)
	}
	otherAccount := newID("inv")
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_accounts
		(id,household_id,kind,name,cash_minor) VALUES(?,?,?,?,?)`,
		otherAccount, otherHousehold, "security", "别人的证券", 999999); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO investment_holdings
		(id,household_id,account_id,symbol,name,asset_class,asset_label,cost_minor,market_value_minor)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		newID("hold"), otherHousehold, otherAccount, "000001", "别人持仓", "stock", "股票", 1, 999999); err != nil {
		t.Fatal(err)
	}

	body := c.mustOK("GET", "/api/v1/accounts?include=investments", nil)
	inv, ok := body["investments"].(map[string]any)
	if !ok {
		t.Fatalf("missing investments projection: %v", body)
	}
	if got := number(inv["account_count"]); got != 2 {
		t.Fatalf("account_count = %d, want 2", got)
	}
	if got := number(inv["holding_count"]); got != 2 {
		t.Fatalf("holding_count = %d, want 2", got)
	}
	if got := number(inv["total_cash_minor"]); got != 500000 {
		t.Fatalf("total_cash_minor = %d, want 500000", got)
	}
	if got := number(inv["total_holdings_value_minor"]); got != 160000 {
		t.Fatalf("total_holdings_value_minor = %d, want 160000", got)
	}
	if got := number(inv["total_market_value_minor"]); got != 660000 {
		t.Fatalf("total_market_value_minor = %d, want 660000", got)
	}
	if got := number(inv["total_cost_minor"]); got != 150000 {
		t.Fatalf("total_cost_minor = %d, want 150000", got)
	}
	if got := number(inv["total_unrealized_pnl_minor"]); got != 10000 {
		t.Fatalf("total_unrealized_pnl_minor = %d, want 10000", got)
	}
	if got := number(inv["total_dividends_minor"]); got != 10000 {
		t.Fatalf("total_dividends_minor = %d, want 10000", got)
	}
	if got := number(inv["total_return_minor"]); got != 20000 {
		t.Fatalf("total_return_minor = %d, want 20000", got)
	}
	if got := number(inv["total_return_bp"]); got != 1333 {
		t.Fatalf("total_return_bp = %d, want 1333", got)
	}
	if got := number(inv["total_dividend_yield_bp"]); got != 666 {
		t.Fatalf("total_dividend_yield_bp = %d, want 666", got)
	}
	if got := number(inv["total_market_value_minor"]); got == 999999+660000 {
		t.Fatal("other household leaked into the totals")
	}

	accounts, _ := inv["accounts"].([]any)
	if len(accounts) != 2 {
		t.Fatalf("accounts = %v, want 2", accounts)
	}
	sec := accounts[0].(map[string]any)
	if sec["name"] != "证券账户" {
		t.Fatalf("accounts[0] = %v, want security account first", sec)
	}
	if got := number(sec["market_value_minor"]); got != 620000 {
		t.Fatalf("security market value = %d, want 620000", got)
	}
	if got := number(sec["return_bp"]); got != 3000 {
		t.Fatalf("security return_bp = %d, want 3000", got)
	}
	if got := number(sec["dividend_yield_bp"]); got != 1000 {
		t.Fatalf("security dividend yield = %d, want 1000", got)
	}
	holdings, _ := sec["holdings"].([]any)
	if len(holdings) != 1 {
		t.Fatalf("security holdings = %v, want 1", holdings)
	}
	holding := holdings[0].(map[string]any)
	if got := number(holding["total_return_minor"]); got != 25000 {
		t.Fatalf("holding total return = %d, want 25000", got)
	}
	if got := number(holding["total_return_bp"]); got != 2500 {
		t.Fatalf("holding total return bp = %d, want 2500", got)
	}
	if holding["last_trade_on"] != "2026-11-01" {
		t.Fatalf("last_trade_on = %v", holding["last_trade_on"])
	}

	byKind, _ := inv["by_kind"].([]any)
	if len(byKind) != 2 {
		t.Fatalf("by_kind = %v, want 2", byKind)
	}
	top, _ := inv["top_holdings"].([]any)
	if len(top) != 2 {
		t.Fatalf("top_holdings = %v, want 2", top)
	}
	if top[0].(map[string]any)["symbol"] != "600900" {
		t.Fatalf("top holding order = %v, want 600900 first", top)
	}

	// Combined projections stay backward compatible.
	combined := c.mustOK("GET", "/api/v1/accounts?include=credit_debt,investments", nil)
	if _, ok := combined["credit_debt"]; !ok {
		t.Fatalf("combined include missing credit_debt: %v", combined)
	}
	if _, ok := combined["investments"]; !ok {
		t.Fatalf("combined include missing investments: %v", combined)
	}
	code, out := c.do("GET", "/api/v1/accounts?include=unknown", nil)
	if code != 400 {
		t.Fatalf("unknown include => %d %v, want 400", code, out)
	}
}
