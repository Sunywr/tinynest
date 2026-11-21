package api

import (
	"fmt"
	"testing"
)

func TestStatementUnpaidPages(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{"name": "Paging card", "kind": "credit_card", "cycle_start_day": 1, "cycle_end_day": 28, "due_day": 20, "due_month_offset": 1})
	for day := 1; day <= 12; day++ {
		date := fmt.Sprintf("2026-01-%02d", day)
		paid := 30
		if day == 12 {
			paid = 100
		}
		_, err := database.Exec(`INSERT INTO credit_statements(id,instrument_id,household_id,cycle_start_on,cycle_end_on,due_on,system_due_minor,paid_minor) SELECT ?,id,household_id,?,?,?,100,? FROM credit_instruments WHERE id=?`, date, date, date, date, paid, card["id"])
		if err != nil {
			t.Fatal(err)
		}
	}
	const path = "/api/v1/credit-statements?unpaid=true&page="
	first := c.mustOK("GET", path+"1", nil)
	second := c.mustOK("GET", path+"2", nil)
	items := first["items"].([]any)
	last := second["items"].([]any)
	if len(items) != 10 || first["has_more"] != true || items[0].(map[string]any)["id"] != "2026-01-11" || len(last) != 1 || last[0].(map[string]any)["id"] != "2026-01-01" || second["has_more"] != false {
		t.Fatal("incorrect filtered page boundaries")
	}
}
