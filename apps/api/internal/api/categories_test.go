package api

import "testing"

func TestDeleteProjectAndCategoryRetainsHistory(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	project := c.mustOK("POST", "/api/v1/projects", map[string]string{"name": "旧项目"})["id"].(string)
	category := c.mustOK("POST", "/api/v1/categories", map[string]string{"name": "旧分类", "kind": "expense"})["id"].(string)
	account := c.mustOK("POST", "/api/v1/accounts", map[string]string{"name": "测试账户", "kind": "cash"})["id"].(string)
	tx := map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"payment_kind": "cash", "account_id": account,
		"splits": []map[string]any{{"category_id": category, "project_id": project, "amount_minor": 100}},
	}
	c.mustOK("POST", "/api/v1/transactions", tx)

	projectPath := "/api/v1/projects/" + project
	categoryPath := "/api/v1/categories/" + category
	c.mustOK("DELETE", categoryPath, nil)
	c.mustOK("DELETE", categoryPath, nil)
	c.mustOK("DELETE", projectPath, nil)
	c.mustOK("DELETE", projectPath, nil)

	if got := c.mustOK("GET", projectPath+"/summary", nil); number(got["net_expense_minor"]) != 100 {
		t.Fatalf("project lost historical expense: %v", got)
	}
	if got := c.mustOK("GET", "/api/v1/projects", nil); number(got["total"]) != 0 {
		t.Fatalf("deleted project remained listed: %v", got)
	}
	for _, row := range c.mustOK("GET", "/api/v1/categories", nil)["items"].([]any) {
		if id := row.(map[string]any)["id"]; id == category {
			t.Fatalf("deleted category remained listed: %v", row)
		}
	}
	if status, _ := c.do("PATCH", categoryPath, map[string]bool{"active": true}); status != 404 {
		t.Fatalf("deleted category could be re-enabled: %d", status)
	}
	if status, _ := c.do("PATCH", projectPath, map[string]string{"status": "active"}); status != 404 {
		t.Fatalf("deleted project could be re-enabled: %d", status)
	}
	if status, _ := c.do("POST", "/api/v1/transactions", tx); status != 400 {
		t.Fatalf("deleted category accepted a new transaction: %d", status)
	}
	tx["splits"] = []map[string]any{{"category_id": c.firstCategory("expense"), "project_id": project, "amount_minor": 100}}
	if status, _ := c.do("POST", "/api/v1/transactions", tx); status != 400 {
		t.Fatalf("deleted project accepted a new transaction: %d", status)
	}
}
