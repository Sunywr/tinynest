package db

import "context"

// DefaultCategory is one row of the built-in category set. The set lives in Go,
// not SQL, so it can be seeded idempotently for a household created at any time
// (first boot, member approval, operator reseed after a purge).
type DefaultCategory struct {
	Code string
	Kind string
	Name string
	Sort int
}

// DefaultCategories is the V2 single-level category set: 16 expense, 4 income
// and 1 dividend income. Stable ids are `cat_<household_id>_<code>`, so
// re-running the seed never duplicates or overwrites user edits.
var DefaultCategories = []DefaultCategory{
	{Code: "food", Kind: "expense", Name: "饮食", Sort: 1},
	{Code: "snack", Kind: "expense", Name: "零食", Sort: 2},
	{Code: "sport", Kind: "expense", Name: "运动", Sort: 3},
	{Code: "clothing", Kind: "expense", Name: "服饰", Sort: 4},
	{Code: "transport", Kind: "expense", Name: "交通", Sort: 5},
	{Code: "living", Kind: "expense", Name: "生活", Sort: 6},
	{Code: "furniture", Kind: "expense", Name: "家具", Sort: 7},
	{Code: "toys", Kind: "expense", Name: "玩具", Sort: 8},
	{Code: "games", Kind: "expense", Name: "游戏", Sort: 9},
	{Code: "fun", Kind: "expense", Name: "娱乐", Sort: 10},
	{Code: "education", Kind: "expense", Name: "教育", Sort: 11},
	{Code: "parenting", Kind: "expense", Name: "育儿", Sort: 12},
	{Code: "work", Kind: "expense", Name: "工作", Sort: 13},
	{Code: "travel", Kind: "expense", Name: "旅行", Sort: 14},
	{Code: "housing", Kind: "expense", Name: "住房", Sort: 15},
	{Code: "car", Kind: "expense", Name: "汽车", Sort: 16},
	{Code: "salary", Kind: "income", Name: "工资", Sort: 1},
	{Code: "reimburse", Kind: "income", Name: "报销", Sort: 2},
	{Code: "redpacket", Kind: "income", Name: "红包", Sort: 3},
	{Code: "interest", Kind: "income", Name: "利息", Sort: 4},
	{Code: "dividend", Kind: "income", Name: "投资分红", Sort: 5},
}

// SeedForHousehold inserts the default categories for one household. It is
// idempotent on both dialects: every row is guarded by an existence check inside
// the caller's transaction, so concurrent calls cannot create duplicates.
func SeedForHousehold(ctx context.Context, q Queryer, householdID string) error {
	for _, category := range DefaultCategories {
		id := "cat_" + householdID + "_" + category.Code
		var existing int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE id=?`, id).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO categories(id, household_id, kind, name, sort) VALUES (?,?,?,?,?)`,
			id, householdID, category.Kind, category.Name, category.Sort); err != nil {
			return err
		}
	}
	return nil
}
