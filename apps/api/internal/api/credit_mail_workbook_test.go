package api

import "testing"

func TestCMBWorkbookUsesCITICColumns(t *testing.T) {
	rows, err := parseCreditWorkbook("cmb", "0123", [][]string{
		{"交易日期", "入账日期", "交易描述", "卡末四位", "结算币种", "结算金额"},
		{"2023-05-01", "2023-05-02", "测试消费", "0123", "人民币", "125.00"},
		{"2023-05-01", "2023-05-02", "其他卡", "1234", "人民币", "99.00"},
	})
	if err != nil || len(rows) != 1 || rows[0].Amount != 12500 {
		t.Fatalf("unexpected CMB workbook result: %v, %v", rows, err)
	}
}

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
		{"2023-08-01", "2023-08-02", "示例饮品店", "0123", "美元", "人民币", "3.00", "20.00"},
		{"2023-08-01", "2023-08-02", "另一张卡", "1234", "人民币", "人民币", "10.00", "10.00"},
	}
	c.mustOK("PUT", "/api/v1/credit-instruments/"+id+"/statement-provision", map[string]any{
		"statement_date": "2023-08-31", "new_charges_minor": 2000,
	})
	checkProvision := func(want int64) {
		t.Helper()
		var provisional, due int64
		err := database.QueryRow(`SELECT p.provision_minor,s.system_due_minor FROM credit_statement_provisions p
			JOIN credit_statements s ON s.id=p.statement_id WHERE s.instrument_id=?`, id).Scan(&provisional, &due)
		if err != nil || provisional != want || due != 2000 {
			t.Fatalf("provisional=%d due=%d err=%v", provisional, due, err)
		}
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
	checkProvision(2000)
	review := got[0].(map[string]any)["review"].([]any)[0].(map[string]any)
	c.headers = map[string]string{"Idempotency-Key": review["key"].(string)}
	category := c.firstCategory("expense")
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 2000, "occurred_on": "2023-08-01", "payment_kind": "credit",
		"credit_instrument_id": id, "merchant": "示例饮品店",
		"splits": []map[string]any{{"category_id": category, "amount_minor": 2000}},
	})
	checkProvision(0)
}
