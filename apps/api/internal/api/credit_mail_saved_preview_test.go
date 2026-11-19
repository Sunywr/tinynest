package api

import (
	"context"
	"database/sql"
	"testing"
	"tinynest/api/internal/credit"
)

func TestSavedCreditPreviewRestoresChoicesAndClears(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	card := c.mustOK("POST", "/api/v1/credit-instruments", map[string]any{
		"name": "光大测试卡", "kind": "credit_card", "issuer": "光大银行", "cycle_start_day": 1, "cycle_end_day": 31, "due_day": 20, "due_month_offset": 1,
	})
	id := card["id"].(string)
	var household string
	if err := database.QueryRow("SELECT household_id FROM credit_instruments WHERE id=?", id).Scan(&household); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/credit-mail/bindings/" + id + "/preview"
	if got := c.mustOK("GET", path, nil); got["statements"] != nil {
		t.Fatal("unexpected preview")
	}
	ctx := context.Background()
	state := savedCreditPreview{SynchronizedAt: "2026-11-19T04:00:00Z", Choices: map[string]string{}, Statements: []everbrightPreviewStatement{
		{Transactions: []credit.EverbrightTransaction{{TransactionDate: "2026-11-18", PostingDate: "2026-11-18", Amount: 900, Description: "测试饮品店"}}},
	}}
	if err := c.mailAPI.reviewEverbrightStatements(ctx, household, id, state.Statements); err != nil {
		t.Fatal(err)
	}
	key := state.Statements[0].Review[0].Key
	if err := database.WithTx(ctx, func(tx *sql.Tx) error { return c.mailAPI.writeSavedCreditPreview(ctx, tx, household, id, state) }); err != nil {
		t.Fatal(err)
	}
	category := c.firstCategory("expense")
	c.mustOK("PATCH", path, map[string]string{"key": key, "category_id": category})
	got := c.mustOK("GET", path, nil)
	if got["synchronized_at"] != state.SynchronizedAt || got["choices"].(map[string]any)[key] != category {
		t.Fatal("saved preview or choice missing")
	}
	if status, _ := c.do("PATCH", path, map[string]string{"key": key, "category_id": "foreign"}); status != 400 {
		t.Fatalf("invalid category accepted: %d", status)
	}
	c.mustOK("DELETE", path, nil)
	got = c.mustOK("GET", path, nil)
	if got["statements"] != nil || got["synchronized_at"] != "" || len(got["choices"].(map[string]any)) != 0 {
		t.Fatal("preview not cleared")
	}
	if n := number(c.mustOK("GET", "/api/v1/transactions", nil)["total"]); n != 0 {
		t.Fatalf("preview modified ledger: %d", n)
	}
}
