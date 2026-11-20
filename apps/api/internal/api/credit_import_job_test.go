package api

import (
	"testing"
	"time"
)

func TestCreditWorkbookJobResultAvailableOnLaterRequest(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "浦发银行测试卡", "kind": "credit_card", "cycle_start_day": 8, "cycle_end_day": 7,
		"due_day": 25, "due_month_offset": 0,
	})["id"].(string)
	path := "/api/v1/credit-mail/bindings/" + card
	started := c.mustOK("POST", path+"/job", map[string]any{
		"kind": "workbook", "workbook": map[string]any{"card_tail": "0123", "rows": [][]string{
			{"交易日期", "记账日期", "交易摘要", "卡号末四位", "交易币种", "交易金额"},
			{"2023-08-01", "2023-08-02", "测试明细", "0123", "人民币", "20.00"},
		}},
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job := c.mustOK("GET", path+"/job", nil)
		if job["id"] != started["id"] {
			t.Fatalf("unexpected job: %v", job)
		}
		switch job["status"] {
		case "completed":
			if statements, ok := c.mustOK("GET", path+"/preview", nil)["statements"].([]any); !ok || len(statements) != 1 {
				t.Fatalf("missing workbook preview: %v", statements)
			}
			return
		case "failed":
			t.Fatalf("workbook failed: %v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("workbook did not complete")
}
