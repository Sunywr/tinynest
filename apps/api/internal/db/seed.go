package db

import "context"

// DefaultCategory is one row of the built-in category set. The set lives in Go,
// not SQL, so it can be seeded idempotently for a household created at any time
// (first boot, member approval, operator reseed after a purge).
type DefaultCategory struct {
	Code  string
	Kind  string
	Name  string
	Sort  int
	// Icon is the flat icon key rendered by the web client. It is stored in the
	// existing categories.emoji column, which already holds either an emoji
	// character or a named icon key.
	Icon string
	// Color is the palette key ("1".."8") used for the category tile.
	Color string
}

// DefaultCategories is the V2 single-level category set: 16 expense, 4 income
// and 1 dividend income. Stable ids are `cat_<household_id>_<code>`, so
// re-running the seed never duplicates or overwrites user edits.
//
// Every entry carries a flat icon and a palette colour so a fresh household
// never shows a grid of blank tiles.
var DefaultCategories = []DefaultCategory{
	{Code: "food", Kind: "expense", Name: "饮食", Sort: 1, Icon: "meal", Color: "1"},
	{Code: "snack", Kind: "expense", Name: "零食", Sort: 2, Icon: "snack", Color: "2"},
	{Code: "sport", Kind: "expense", Name: "运动", Sort: 3, Icon: "fitness", Color: "3"},
	{Code: "clothing", Kind: "expense", Name: "服饰", Sort: 4, Icon: "clothing", Color: "4"},
	{Code: "transport", Kind: "expense", Name: "交通", Sort: 5, Icon: "bus", Color: "5"},
	{Code: "living", Kind: "expense", Name: "生活", Sort: 6, Icon: "groceries", Color: "6"},
	{Code: "furniture", Kind: "expense", Name: "家具", Sort: 7, Icon: "furniture", Color: "7"},
	{Code: "toys", Kind: "expense", Name: "玩具", Sort: 8, Icon: "toys", Color: "8"},
	{Code: "games", Kind: "expense", Name: "游戏", Sort: 9, Icon: "games", Color: "1"},
	{Code: "fun", Kind: "expense", Name: "娱乐", Sort: 10, Icon: "fun", Color: "2"},
	{Code: "education", Kind: "expense", Name: "教育", Sort: 11, Icon: "study", Color: "3"},
	{Code: "parenting", Kind: "expense", Name: "育儿", Sort: 12, Icon: "baby", Color: "4"},
	{Code: "work", Kind: "expense", Name: "工作", Sort: 13, Icon: "office", Color: "5"},
	{Code: "travel", Kind: "expense", Name: "旅行", Sort: 14, Icon: "flight", Color: "6"},
	{Code: "housing", Kind: "expense", Name: "住房", Sort: 15, Icon: "housing", Color: "7"},
	{Code: "car", Kind: "expense", Name: "汽车", Sort: 16, Icon: "car", Color: "8"},
	{Code: "salary", Kind: "income", Name: "工资", Sort: 1, Icon: "salary", Color: "5"},
	{Code: "reimburse", Kind: "income", Name: "报销", Sort: 2, Icon: "receipt", Color: "6"},
	{Code: "redpacket", Kind: "income", Name: "红包", Sort: 3, Icon: "redpacket", Color: "2"},
	{Code: "interest", Kind: "income", Name: "利息", Sort: 4, Icon: "savings", Color: "3"},
	{Code: "dividend", Kind: "income", Name: "投资分红", Sort: 5, Icon: "investment", Color: "6"},
}

// SeedForHousehold inserts the default categories for one household. It is
// idempotent on both dialects: an existing row is updated in place so a
// household seeded by an older binary picks up the current icon and colour
// without losing a user-renamed category.
func SeedForHousehold(ctx context.Context, q Queryer, householdID string) error {
	for _, category := range DefaultCategories {
		id := "cat_" + householdID + "_" + category.Code
		var existing int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE id=?`, id).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			if _, err := q.ExecContext(ctx,
				`UPDATE categories SET emoji=?, color=? WHERE id=? AND household_id=? AND (emoji='' OR color='')`,
				category.Icon, category.Color, id, householdID); err != nil {
				return err
			}
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO categories(id, household_id, kind, name, sort, emoji, color) VALUES (?,?,?,?,?,?,?)`,
			id, householdID, category.Kind, category.Name, category.Sort, category.Icon, category.Color); err != nil {
			return err
		}
	}
	return nil
}
