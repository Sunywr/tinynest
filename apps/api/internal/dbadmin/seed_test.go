package dbadmin

import (
	"context"
	"testing"
)

// An existing household seeded by an older binary has default categories with
// no icon key and no colour. Re-seeding must fill those in without touching the
// user-editable name, and must not duplicate rows.
func TestSeedCategoriesBackfillsIconAndColor(t *testing.T) {
	admin := newTestAdmin(t)
	ctx := context.Background()

	mustExec(t, admin, `INSERT INTO households (id, name) VALUES (?, ?)`, "hh1", "我的小家")
	mustExec(t, admin, `INSERT INTO categories (id, household_id, kind, name, sort) VALUES (?,?,?,?,?)`,
		"cat_hh1_food", "hh1", "expense", "吃饭", 1)

	households, err := admin.SeedCategories(ctx, "")
	if err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	if len(households) != 1 || households[0] != "hh1" {
		t.Fatalf("seeded households = %v, want [hh1]", households)
	}

	var emoji, color, name string
	if err := admin.DB.QueryRowContext(ctx,
		`SELECT emoji, color, name FROM categories WHERE id=?`, "cat_hh1_food").
		Scan(&emoji, &color, &name); err != nil {
		t.Fatal(err)
	}
	if emoji != "meal" || color != "1" {
		t.Fatalf("backfilled style = %q/%q, want meal/1", emoji, color)
	}
	if name != "吃饭" {
		t.Fatalf("re-seeding overwrote the user-visible name: %q", name)
	}

	var total int
	if err := admin.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE household_id=?`, "hh1").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 21 {
		t.Fatalf("seeded category count = %d, want 21", total)
	}

	// Running it twice is a no-op.
	if _, err := admin.SeedCategories(ctx, ""); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if err := admin.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE household_id=?`, "hh1").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 21 {
		t.Fatalf("second seed changed the row count to %d", total)
	}
}
