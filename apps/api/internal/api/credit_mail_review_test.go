package api

import (
	"context"
	"testing"
	"tinynest/api/internal/credit"
)

func TestEverbrightReviewReconcilesExistingLedger(t *testing.T) {
	ts, c, db := newTestServer(t)
	defer db.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "光大测试卡", "kind": "credit_card", "issuer": "光大银行", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})
	actualID := card["id"].(string)
	var household string
	if err := db.QueryRow("SELECT household_id FROM credit_instruments WHERE id=?", actualID).Scan(&household); err != nil {
		t.Fatal(err)
	}
	row := credit.EverbrightTransaction{TransactionDate: "2026-11-04", PostingDate: "2026-11-05", CardTail: "0123", Description: "测试消费", Amount: 1234}
	statements := []everbrightPreviewStatement{{Transactions: []credit.EverbrightTransaction{row, row}}}
	if err := c.mailAPI.reviewEverbrightStatements(context.Background(), household, actualID, statements); err != nil {
		t.Fatal(err)
	}
	first, second := statements[0].Review[0].Key, statements[0].Review[1].Key
	if first == second {
		t.Fatal("identical legitimate rows lost distinct keys")
	}
	c.headers = map[string]string{"Idempotency-Key": first}
	category := c.firstCategory("expense")
	created := c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 1234, "occurred_on": row.TransactionDate, "payment_kind": "credit",
		"credit_instrument_id": actualID, "merchant": row.Description,
		"splits": []map[string]any{{"category_id": category, "amount_minor": 1234}},
	})
	c.headers = nil
	statements[0].Review = nil
	if err := c.mailAPI.reviewEverbrightStatements(context.Background(), household, actualID, statements); err != nil {
		t.Fatal(err)
	}
	got := statements[0].Review
	if !got[0].Imported || got[1].Imported || got[1].Key != second || got[1].SuggestedCategory != category ||
		len(got[1].Matches) != 1 || got[1].Matches[0].ID != created["id"] {
		t.Fatalf("unexpected reviews: %+v", got)
	}
	c.mustOK("DELETE", "/api/v1/transactions/"+created["id"].(string), nil)
	statements[0].Review = nil
	if err := c.mailAPI.reviewEverbrightStatements(context.Background(), household, actualID, statements); err != nil {
		t.Fatal(err)
	}
	if !statements[0].Review[0].Imported || len(statements[0].Review[0].Matches) != 0 {
		t.Fatal("reversed import should retain deduplication and disappear from active matches")
	}
}
