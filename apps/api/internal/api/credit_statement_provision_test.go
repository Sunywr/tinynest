package api

import (
	"database/sql"
	"net/http/httptest"
	"testing"

	"tinynest/api/internal/credit"
	"tinynest/api/internal/money"
)

func TestMailProvisionRepeatAndConflictingAmounts(t *testing.T) {
	ts, c, db := newTestServer(t)
	defer db.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "摘要测试", "kind": "credit_card", "cycle_start_day": 1, "cycle_end_day": 31,
		"due_day": 20, "due_month_offset": 1,
	})["id"].(string)
	var p Principal
	if err := db.QueryRow("SELECT household_id FROM credit_instruments WHERE id=?", card).Scan(&p.HouseholdID); err != nil {
		t.Fatal(err)
	}
	item := func(value money.Minor) everbrightPreviewStatement {
		return everbrightPreviewStatement{Provision: "eligible", Amounts: &credit.StatementAmounts{
			StatementDate: "2023-08-31", NewCharges: &value,
		}}
	}
	apply := func(items ...everbrightPreviewStatement) []everbrightPreviewStatement {
		t.Helper()
		r := httptest.NewRequest("POST", "/", nil)
		if err := c.mailAPI.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
			return c.mailAPI.applyMailProvisions(tx, r, p, card, items)
		}); err != nil {
			t.Fatal(err)
		}
		return items
	}
	if got := apply(item(100), item(200)); got[0].Provision != "review" || got[1].Provision != "review" {
		t.Fatal("conflicting summaries were accepted")
	}
	for range 2 {
		if got := apply(item(100)); got[0].Provision != "recorded" {
			t.Fatal("valid summary did not reconcile")
		}
	}
	if got := apply(item(200)); got[0].Provision != "review" {
		t.Fatal("different summary overwrote recorded amount")
	}
	var count, amount int
	if err := db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(i.amount_minor),0) FROM credit_statement_items i
		JOIN credit_statements s ON s.id=i.statement_id WHERE s.instrument_id=? AND i.source_type='statement_provision'`, card).
		Scan(&count, &amount); err != nil || count != 1 || amount != 100 {
		t.Fatalf("count=%d amount=%d err=%v", count, amount, err)
	}
}

func TestStatementProvisionTracksUncoveredDetail(t *testing.T) {
	ts, c, db := newTestServer(t)
	defer db.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "账单暂记测试", "kind": "credit_card", "cycle_start_day": 1, "cycle_end_day": 31,
		"due_day": 20, "due_month_offset": 1,
	})["id"].(string)
	provision := func() {
		c.mustOK("PUT", "/api/v1/credit-instruments/"+card+"/statement-provision", map[string]any{
			"statement_date": "2023-08-31", "new_charges_minor": 10000, "amount_due_minor": 15000,
		})
	}
	check := func(want int64, wantItems int) {
		t.Helper()
		var remaining, ledgerDue, bankDue int64
		var items int
		err := db.QueryRow(`SELECT p.provision_minor,s.system_due_minor,s.platform_amount_minor,
			(SELECT COUNT(*) FROM credit_statement_items i WHERE i.statement_id=s.id AND i.source_type='statement_provision')
			FROM credit_statement_provisions p JOIN credit_statements s ON s.id=p.statement_id WHERE s.instrument_id=?`, card).
			Scan(&remaining, &ledgerDue, &bankDue, &items)
		if err != nil || remaining != want || ledgerDue != 10000 || bankDue != 15000 || items != wantItems {
			t.Fatalf("remaining=%d ledger=%d bank=%d items=%d err=%v", remaining, ledgerDue, bankDue, items, err)
		}
	}
	category := c.firstCategory("expense")
	detail := func(amount int) string {
		return c.mustOK("POST", "/api/v1/transactions", map[string]any{
			"type": "expense", "amount_minor": amount, "occurred_on": "2023-08-20",
			"payment_kind": "credit", "credit_instrument_id": card,
			"splits": []map[string]any{{"category_id": category, "amount_minor": amount}},
		})["id"].(string)
	}
	provision()
	provision()
	check(10000, 1)
	detail(3000)
	check(7000, 3)
	id := detail(7000)
	check(0, 4)
	c.mustOK("DELETE", "/api/v1/transactions/"+id, nil)
	check(7000, 5)
}
