package api

import "testing"

func TestDeleteUnusedCreditInstrument(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "测试信用卡", "kind": "credit_card", "issuer": "工商银行",
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})
	cardID := card["id"].(string)
	path := "/api/v1/credit-instruments/" + cardID
	list := c.mustOK("GET", "/api/v1/credit-instruments", nil)
	if issuer := list["items"].([]any)[0].(map[string]any)["issuer"]; issuer != "工商银行" {
		t.Fatalf("missing issuer: %v", list)
	}
	post := func(id string) (int, map[string]any) {
		return c.do("POST", "/api/v1/transactions", map[string]any{
			"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
			"payment_kind": "credit", "credit_instrument_id": id,
			"splits": []map[string]any{{"category_id": c.firstCategory("expense"), "amount_minor": 100}},
		})
	}
	c.mustOK("DELETE", path, nil)
	c.mustOK("DELETE", path, nil)
	if list := c.mustOK("GET", "/api/v1/credit-instruments", nil); number(list["total"]) != 0 {
		t.Fatalf("deleted instrument visible: %v", list)
	}
	if status, _ := c.do("DELETE", "/api/v1/credit-instruments/missing", nil); status != 404 {
		t.Fatalf("missing instrument deletion returned %d", status)
	}
	if status, _ := post(cardID); status != 400 {
		t.Fatalf("deleted instrument accepted new payment: %d", status)
	}
	if status, _ := c.do("PATCH", path, map[string]string{"status": "active"}); status != 400 {
		t.Fatalf("deleted instrument could be edited: %d", status)
	}
	used := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "已入账信用卡", "kind": "credit_card",
		"cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})["id"].(string)
	if status, body := post(used); status != 201 {
		t.Fatalf("credit expense returned %d: %v", status, body)
	}
	if status, _ := c.do("DELETE", "/api/v1/credit-instruments/"+used, nil); status != 409 {
		t.Fatalf("used instrument deletion returned %d", status)
	}
	if limit := c.mustOK("GET", "/api/v1/credit-instruments/"+used+"/limit", nil); number(limit["outstanding_minor"]) != 100 {
		t.Fatalf("used instrument balance changed: %v", limit)
	}

}
