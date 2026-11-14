package api

import "testing"

func TestCategoryMaintenance(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	category := client.mustOK("POST", "/api/v1/categories", map[string]any{"kind": "expense", "name": "旧名称"})
	id := category["id"].(string)
	path := "/api/v1/categories/" + id
	client.mustOK("PATCH", path, map[string]any{"name": "新名称", "sort": 7, "active": false})
	client.mustOK("PATCH", path, map[string]any{"sort": 0})
	items := client.mustOK("GET", "/api/v1/categories", nil)["items"].([]any)
	for _, item := range items {
		row := item.(map[string]any)
		if row["id"] == id && row["name"] == "新名称" && number(row["sort"]) == 0 && number(row["active"]) == 0 {
			return
		}
	}
	t.Fatal("category update missing or partial update replaced another field")
}
