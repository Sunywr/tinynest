package api

import (
	"context"
	"testing"
)

func TestCategoriesAreFlat(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	parent := c.firstCategory("expense")
	status, _ := c.do("POST", "/api/v1/categories", map[string]string{
		"name": "二级分类", "kind": "expense", "parent_id": parent,
	})
	if status != 400 {
		t.Errorf("nested category creation returned %d", status)
	}

	ctx := context.Background()
	const legacy = "legacy-child"
	if _, err := database.ExecContext(ctx, `INSERT INTO categories(id,household_id,parent_id,kind,name,level)
        SELECT ?,household_id,id,kind,?,2 FROM categories WHERE id=?`, legacy, "旧分类", parent); err != nil {
		t.Fatal(err)
	}
	account := c.mustOK("POST", "/api/v1/accounts", map[string]string{"name": "测试账户", "kind": "cash"})["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"payment_kind": "cash", "account_id": account,
		"splits": []map[string]any{{"category_id": legacy, "amount_minor": 100}},
	})
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var flat int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE id=? AND parent_id IS NULL AND level=1`, legacy).Scan(&flat); err != nil || flat != 1 {
		t.Fatalf("legacy category not flattened: count=%d err=%v", flat, err)
	}
	ranking := c.mustOK("GET", "/api/v1/dashboard/categories?from=2026-11-01&to=2026-11-30", nil)["items"].([]any)
	if len(ranking) != 1 {
		t.Fatalf("expected one ranked category: %v", ranking)
	}
	row := ranking[0].(map[string]any)
	if row["category_id"] != legacy || row["name"] != "旧分类" || number(row["expense_minor"]) != 100 {
		t.Fatalf("legacy category lost its historical expense: %v", row)
	}
}
