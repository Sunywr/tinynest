package api

import (
	"strings"
	"testing"
)

// Seeded default categories must carry a flat icon key and a palette colour, so
// a brand-new household never renders a grid of blank tiles.
func TestDefaultCategoriesCarryIconAndColor(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	items := v012Rows(t, c.mustOK("GET", "/api/v1/categories", nil))
	if len(items) != 21 {
		t.Fatalf("expected 21 default categories, got %d", len(items))
	}
	for _, item := range items {
		emoji, _ := item["emoji"].(string)
		color, _ := item["color"].(string)
		if strings.TrimSpace(emoji) == "" {
			t.Fatalf("default category %v has no icon key", item["name"])
		}
		if !validCategoryColor(color) || color == "" {
			t.Fatalf("default category %v has colour %q, want 1..8", item["name"], color)
		}
	}
	// Icons must be resolvable keys rather than stray characters: the seed uses
	// the same named keys the web client maps to flat icons.
	for _, code := range []string{"meal", "bus", "salary"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM categories WHERE emoji=?`, code).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("expected a default category seeded with icon key %q", code)
		}
	}
}

// Creating and editing a category must round-trip the chosen colour and reject
// anything outside the palette.
func TestCategoryColorRoundTrip(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	created := c.mustOK("POST", "/api/v1/categories", map[string]any{
		"kind": "expense", "name": "宠物", "emoji": "family", "color": "3", "sort": 40,
	})
	if created["color"] != "3" {
		t.Fatalf("create returned colour %v, want 3", created["color"])
	}
	id := created["id"].(string)

	// Listing must expose the colour.
	found := false
	for _, item := range v012Rows(t, c.mustOK("GET", "/api/v1/categories", nil)) {
		if item["id"] == id {
			found = true
			if item["color"] != "3" {
				t.Fatalf("list showed colour %v, want 3", item["color"])
			}
		}
	}
	if !found {
		t.Fatal("created category missing from the list")
	}

	// Patching the colour changes it without touching the icon.
	c.mustOK("PATCH", "/api/v1/categories/"+id, map[string]any{"color": "7"})
	for _, item := range v012Rows(t, c.mustOK("GET", "/api/v1/categories", nil)) {
		if item["id"] == id {
			if item["color"] != "7" || item["emoji"] != "family" {
				t.Fatalf("after patch colour=%v icon=%v, want 7/family", item["color"], item["emoji"])
			}
		}
	}

	// Out-of-palette colours are rejected on both create and update.
	if status, _ := c.do("POST", "/api/v1/categories", map[string]any{
		"kind": "expense", "name": "非法颜色", "color": "99",
	}); status != 400 {
		t.Fatalf("invalid colour accepted on create: %d", status)
	}
	if status, _ := c.do("PATCH", "/api/v1/categories/"+id, map[string]any{"color": "red"}); status != 400 {
		t.Fatalf("invalid colour accepted on update: %d", status)
	}
}

// Transactions and the category ranking must expose the colour so the bills list
// and the donut stay consistent with the entry grid.
func TestCategoryColorFlowsIntoListsAndStats(t *testing.T) {
	ts, c, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()

	category := c.mustOK("POST", "/api/v1/categories", map[string]any{
		"kind": "expense", "name": "交通补充", "emoji": "bus", "color": "5", "sort": 41,
	})
	account := c.mustOK("POST", "/api/v1/accounts", map[string]string{"kind": "cash", "name": "现金"})
	c.mustOK("POST", "/api/v1/transactions", map[string]any{
		"type": "expense", "amount_minor": 2500, "occurred_on": "2026-11-20",
		"category_id": category["id"], "account_id": account["id"], "note": "地铁月票",
	})

	items := v012Rows(t, c.mustOK("GET", "/api/v1/transactions?from=2026-11-01&to=2026-11-30", nil))
	if len(items) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(items))
	}
	if items[0]["category_color"] != "5" {
		t.Fatalf("transaction category_color = %v, want 5", items[0]["category_color"])
	}

	stats := v012Rows(t, c.mustOK("GET", "/api/v1/stats/categories?from=2026-11-01&to=2026-11-30&kind=expense", nil))
	if len(stats) != 1 {
		t.Fatalf("expected 1 ranking row, got %d", len(stats))
	}
	if stats[0]["color"] != "5" {
		t.Fatalf("ranking colour = %v, want 5", stats[0]["color"])
	}
}
