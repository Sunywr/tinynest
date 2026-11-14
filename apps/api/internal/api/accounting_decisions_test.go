package api

import (
	"net/http"
	"testing"
)

func TestCrossYearCashRefundKeepsProjectAndCashflowSeparate(t *testing.T) {
	ts, c, database := newTestServer(t)
	t.Cleanup(func() {
		ts.Close()
		database.Close()
	})
	cash := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "退款测试", "opening_balance_minor": 0,
	})["id"].(string)
	project := c.mustOK("POST", "/api/v1/projects", map[string]any{
		"name": "跨年项目",
	})["id"].(string)
	transaction := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 10_000, "occurred_on": "2025-12-31",
		"payment_kind": "cash", "account_id": cash,
		"splits": []any{map[string]any{
			"project_id": project, "category_id": c.firstCategory("expense"), "amount_minor": 10_000,
		}},
	})["id"].(string)

	for _, refund := range []struct {
		date   string
		amount int64
	}{{"2026-01-01", 2_000}, {"2026-02-01", 3_000}} {
		c.mustOK("POST", "/api/v1/refunds", map[string]any{
			"transaction_id": transaction, "amount_minor": refund.amount, "refund_on": refund.date,
			"destination_type": "cash", "destination_id": cash,
		})
	}
	status, body := c.do("POST", "/api/v1/refunds", map[string]any{
		"transaction_id": transaction, "amount_minor": 5_001, "refund_on": "2026-02-02",
		"destination_type": "cash", "destination_id": cash,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("over-refund status = %d, want 400: %v", status, body)
	}
	summary := c.mustOK("GET", "/api/v1/projects/"+project+"/summary", nil)
	if number(summary["expense_minor"]) != 10_000 ||
		number(summary["refunded_minor"]) != 5_000 ||
		number(summary["net_expense_minor"]) != 5_000 {
		t.Errorf("project summary does not reflect both refunds: %v", summary)
	}
	for _, period := range []struct {
		from, to string
		want     int64
	}{
		{"2025-12-01", "2025-12-31", -10_000},
		{"2026-01-01", "2026-01-31", 2_000},
		{"2026-02-01", "2026-02-28", 3_000},
	} {
		path := "/api/v1/dashboard/summary?from=" + period.from + "&to=" + period.to
		report := c.mustOK("GET", path, nil)
		if got := number(report["cashflow_minor"]); got != period.want {
			t.Errorf("cashflow for %s through %s = %d, want %d", period.from, period.to, got, period.want)
		}
	}
}

func TestInvestmentValuationOmissionAndClosedPosition(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "估值测试", "opening_balance_minor": 100_000,
	})["id"].(string)
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "VALUATION", "name": "测试资产", "kind": "stock", "account_id": account,
	})["id"].(string)
	trade := func(side, date string, amount int64) {
		t.Helper()
		c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
			"asset_id": asset, "side": side, "trade_on": date, "quantity_scaled": 1_000_000,
			"gross_minor": amount, "fee_minor": 0, "from_account_id": account,
		})
	}
	dashboard := func() map[string]any {
		return c.mustOK("GET", "/api/v1/dashboard/summary?from=2026-11-01&to=2026-11-30", nil)
	}
	trade("buy", "2026-11-02", 10_000)
	if summary := dashboard(); number(summary["investment_unrealized_minor"]) != 0 {
		t.Errorf("missing valuation must not generate a loss: %v", summary)
	}
	c.mustOK("POST", "/api/v1/investments/valuations", map[string]any{
		"asset_id": asset, "valued_on": "2026-11-03", "market_value_minor": 20_000,
	})
	trade("sell", "2026-11-04", 13_000)
	position := findRow(t, c.mustOK("GET", "/api/v1/investments/positions", nil), "asset_id", asset)
	if position["unrealized_pnl_minor"] != float64(0) || number(position["total_return_minor"]) != 3_000 ||
		number(position["reference_return_bps"]) != 3_000 {
		t.Errorf("closed position must show realized gain and reference return: %v", position)
	}
	if summary := dashboard(); number(summary["investment_total_return_minor"]) != 3_000 {
		t.Errorf("stale valuation must not inflate dashboard return after sellout: %v", summary)
	}
}

func TestAuditLogReadIsHouseholdScoped(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	if _, err := database.Exec(`
		INSERT INTO audit_logs(household_id, actor, action, entity, entity_id)
		VALUES ('other-household','other-admin','create_account','account','other-account')`); err != nil {
		t.Fatal(err)
	}
	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "cash", "name": "审计测试", "opening_balance_minor": 0,
	})["id"].(string)
	logs := c.mustOK("GET", "/api/v1/admin/audit-logs?limit=100", nil)
	row := findRow(t, logs, "entity_id", account)
	if row["entity"] != "account" {
		t.Errorf("unexpected audit metadata: %v", row)
	}
	for _, raw := range logs["items"].([]any) {
		entry := raw.(map[string]any)
		if entry["entity_id"] == "other-account" {
			t.Fatal("audit query returned another household's event")
		}
	}
}

func TestInvestmentFeeRuleUsesTradeDate(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	account := c.mustOK("POST", "/api/v1/accounts", map[string]any{
		"kind": "securities", "name": "费率测试", "opening_balance_minor": 1_000_000,
	})["id"].(string)
	for _, rule := range []struct {
		date string
		bps  int64
	}{{"2026-11-01", 10}, {"2026-12-01", 20}} {
		c.mustOK("POST", "/api/v1/investments/fee-rules", map[string]any{
			"account_id": account, "effective_from": rule.date, "commission_bps": rule.bps,
		})
	}
	asset := c.mustOK("POST", "/api/v1/investments/assets", map[string]any{
		"code": "FEE", "name": "费率资产", "kind": "stock", "account_id": account,
	})["id"].(string)
	for _, trade := range []struct {
		date string
		fee  int64
	}{{"2026-10-31", 0}, {"2026-11-30", 100}, {"2026-12-01", 200}} {
		body := c.mustOK("POST", "/api/v1/investments/trades", map[string]any{
			"asset_id": asset, "side": "buy", "trade_on": trade.date,
			"quantity_scaled": 1_000_000, "gross_minor": 100_000, "from_account_id": account,
		})
		var fee int64
		if err := database.QueryRow("SELECT fee_minor FROM investment_trades WHERE id=?", body["id"]).Scan(&fee); err != nil {
			t.Fatal(err)
		}
		if fee != trade.fee {
			t.Errorf("fee on %s = %d, want %d", trade.date, fee, trade.fee)
		}
	}
}
