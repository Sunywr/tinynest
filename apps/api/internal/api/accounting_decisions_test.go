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
