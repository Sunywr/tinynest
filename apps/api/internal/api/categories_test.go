package api

import "testing"

// V2 categories are single-level: the schema itself has no parent_id/level, so
// there is no hierarchy to flatten and no migration-time UPDATE any more.
func TestCategoriesAreSingleLevel(t *testing.T) {
	ts, _, database := newTestServer(t)
	defer ts.Close()
	defer database.Close()

	// No parent_id / level columns exist at all. Query the active dialect's
	// catalog so the same regression runs on SQLite and MySQL.
	columns := map[string]bool{}
	if database.IsMySQL() {
		rows, err := database.Query(`SELECT column_name FROM information_schema.columns
			WHERE table_schema=DATABASE() AND table_name='categories'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			columns[name] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	} else {
		rows, err := database.Query(`PRAGMA table_info(categories)`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull, pk int
			var dflt any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				t.Fatal(err)
			}
			columns[name] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	if columns["parent_id"] || columns["level"] {
		t.Fatalf("categories must not have parent_id/level, got %v", columns)
	}
	for _, want := range []string{"id", "household_id", "kind", "name", "emoji", "sort", "active", "created_at"} {
		if !columns[want] {
			t.Fatalf("categories missing column %q: %v", want, columns)
		}
	}

	// Seeded defaults are 16 expense + 4 income + 1 dividend, all level-1.
	var expense, income int
	if err := database.QueryRow(`SELECT
		SUM(CASE WHEN kind='expense' THEN 1 ELSE 0 END),
		SUM(CASE WHEN kind='income' THEN 1 ELSE 0 END) FROM categories`).Scan(&expense, &income); err != nil {
		t.Fatal(err)
	}
	if expense != 16 || income != 5 {
		t.Fatalf("default category counts = expense %d / income %d, want 16/5", expense, income)
	}
}

func TestDeleteCategoryRetainsHistory(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	category := c.mustOK("POST", "/api/v1/categories", map[string]string{"name": "临时分类", "kind": "expense"})["id"].(string)
	account := c.mustOK("POST", "/api/v1/accounts", map[string]string{"name": "测试账户", "kind": "cash"})["id"].(string)
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-14",
		"category_id": category, "account_id": account,
	})

	categoryPath := "/api/v1/categories/" + category
	c.mustOK("DELETE", categoryPath, nil)
	c.mustOK("DELETE", categoryPath, nil) // idempotent soft delete
	for _, row := range v012Rows(t, c.mustOK("GET", "/api/v1/categories", nil)) {
		if row["id"] == category {
			t.Fatalf("deleted category remained listed: %v", row)
		}
	}
	if status, _ := c.do("PATCH", categoryPath, map[string]bool{"active": true}); status != 404 {
		t.Fatalf("deleted category could be re-enabled: %d", status)
	}
	// A deleted category cannot be used for a NEW transaction...
	if status, _ := c.do("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 100, "occurred_on": "2026-11-15",
		"category_id": category, "account_id": account,
	}); status != 400 {
		t.Fatalf("deleted category accepted a new transaction: %d", status)
	}
	// ...but the historical flow and its category name still resolve.
	stats := c.mustOK("GET", "/api/v1/stats/categories?from=2026-11-01&to=2026-11-30", nil)
	items := v012Rows(t, stats)
	if len(items) != 1 || items[0]["category_id"] != category || items[0]["name"] != "临时分类" || number(items[0]["amount_minor"]) != 100 {
		t.Fatalf("historical category lost from stats: %v", items)
	}
}
