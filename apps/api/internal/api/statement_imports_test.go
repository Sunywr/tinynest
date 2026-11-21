package api

import (
	"strings"
	"testing"
)

func TestStatementImportsMixedHousehold(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := func(name string) string {
		return c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
			"name": name, "kind": "credit_card", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
		})["id"].(string)
	}
	firstCard, secondCard := card("家庭信用卡一"), card("家庭信用卡二")
	category := c.firstCategory("expense")
	manual := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 4800, "occurred_on": "2025-10-10", "payment_kind": "credit",
		"credit_instrument_id": firstCard, "merchant": "小家餐厅", "note": "午餐",
		"splits": []splitInput{{CategoryID: category, AmountMinor: 4800}},
	})["id"]
	const path = "/api/v1/statement-imports"
	const reference = "202510101234567890"
	stage := func(source, profile, merchant, ref string, copies int) {
		t.Helper()
		items := make([]statementImportData, copies)
		for i := range items {
			items[i] = statementImportData{Day: "2025-10-10", Amount: 4800, Merchant: merchant, Kind: "expense",
				Method: "信用卡(0123)", Reference: ref}
		}
		c.mustOK("POST", path, map[string]any{"source": source, "profile": profile, "file": strings.Repeat("a", 64), "rows": items})
	}
	list := func() []any { return c.mustOK("GET", path, nil)["items"].([]any) }
	locate := func(profile string) string {
		t.Helper()
		for _, value := range list() {
			item := value.(map[string]any)
			if item["profile"] == profile {
				return item["id"].(string)
			}
		}
		t.Fatalf("missing profile %q", profile)
		return ""
	}
	resolve := func(id, card, action string) map[string]any {
		t.Helper()
		return c.mustOK("PUT", path+"/"+id, map[string]any{"action": action, "card_id": card, "category_id": category})
	}
	stage("wechat", "成员一微信", "小家餐厅", reference, 1)
	stage("wechat", "成员一微信", "小家餐厅", reference, 1)
	if len(list()) != 1 {
		t.Fatal("duplicate statement row survived upload")
	}
	wechat := locate("成员一微信")
	for range 2 {
		if match := resolve(wechat, firstCard, "auto"); match["transaction_id"] != manual || match["resolution"] != "linked" {
			t.Fatalf("manual entry was not retained: %v", match)
		}
	}
	stage("credit", "成员一卡", "微信支付", reference, 1)
	if match := resolve(locate("成员一卡"), firstCard, "auto"); match["transaction_id"] != manual {
		t.Fatalf("shared reference did not match: %v", match)
	}
	stage("credit", "成员二卡", "小家餐厅", reference, 1)
	if posted := resolve(locate("成员二卡"), secondCard, "auto"); posted["transaction_id"] == manual || posted["resolution"] != "posted" {
		t.Fatalf("different card was not posted independently: %v", posted)
	}
	stage("alipay", "支付宝无编号", "小家餐厅", "", 2)
	stage("alipay", "支付宝无编号", "小家餐厅", "", 2)
	if len(list()) != 5 {
		t.Fatal("identical lines in a file were collapsed or upload was repeated")
	}
	stage("wechat", "摘要不明确", "支付", "", 1)
	ambiguous := locate("摘要不明确")
	code, body := c.do("PUT", path+"/"+ambiguous, map[string]any{
		"action": "auto", "card_id": firstCard, "category_id": category,
	})
	if code != 409 {
		t.Fatalf("same amount without merchant evidence was accepted: %d %v", code, body)
	}
	if match := resolve(wechat, firstCard, "unlink"); match["resolution"] != "" {
		t.Fatalf("unlink failed: %v", match)
	}
	var count int
	var total int64
	if err := database.QueryRow("SELECT COUNT(*),COALESCE(SUM(amount_minor),0) FROM transactions WHERE type='expense'").Scan(&count, &total); err != nil || count != 2 || total != 9600 {
		t.Fatalf("count=%d total=%d err=%v", count, total, err)
	}
	detail := c.mustOK("GET", "/api/v1/transactions/"+manual.(string), nil)
	if detail["note"] != "午餐" {
		t.Fatalf("manual note was not preserved: %v", detail)
	}
	if _, err := database.Exec("UPDATE users SET role='viewer' WHERE username='admin'"); err != nil {
		t.Fatal(err)
	}
	code, _ = c.do("PUT", path+"/"+ambiguous, map[string]any{"action": "skip"})
	if code != 403 {
		t.Fatalf("read-only member could mutate import state: %d", code)
	}
}
