package api

import "testing"

func TestCreditWorkbookPreviewUsesSettlementAndCardTail(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	id := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "中信银行测试卡", "kind": "credit_card", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})["id"].(string)
	rows := [][]string{
		{"本期账单明细(人民币)"},
		{"交易日期", "入账日期", "交易描述", "卡末四位", "交易币种", "结算币种", "交易金额", "结算金额"},
		{"2026-11-01", "2026-11-02", "示例饮品店", "0123", "美元", "人民币", "3.00", "20.00"},
		{"2026-11-01", "2026-11-02", "另一张卡", "1234", "人民币", "人民币", "10.00", "10.00"},
	}
	path := "/api/v1/credit-mail/bindings/" + id
	input := map[string]any{"card_tail": "0123", "rows": rows}
	for range 2 {
		c.mustOK("POST", path+"/workbook", input)
	}
	got := c.mustOK("GET", path+"/preview", nil)["statements"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected one statement, got %d", len(got))
	}
	details := got[0].(map[string]any)["transactions"].([]any)
	if len(details) != 1 || number(details[0].(map[string]any)["Amount"]) != 2000 {
		t.Fatalf("invalid worksheet result: %v", details)
	}
	if number(c.mustOK("GET", "/api/v1/transactions", nil)["total"]) != 0 {
		t.Fatal("preview posted a transaction")
	}
}
