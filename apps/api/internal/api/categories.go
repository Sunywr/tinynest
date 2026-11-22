package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"tinynest/api/internal/db"
)

// category is the public shape of one single-level category.
type category struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Emoji  string `json:"emoji"`
	Sort   int    `json:"sort"`
	Active int    `json:"active"`
}

func validCategoryEmoji(emoji string) bool {
	return utf8.RuneCountInString(emoji) <= 16 && strings.IndexFunc(emoji, unicode.IsControl) < 0
}

func validCategoryName(name string) bool {
	length := utf8.RuneCountInString(name)
	return length >= 1 && length <= 50
}

func (a *API) handleListCategories(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	query := `SELECT id,kind,name,emoji,sort,active FROM categories
		WHERE household_id=? AND active>=0`
	args := []any{p.HouseholdID}
	if kind := strings.TrimSpace(r.URL.Query().Get("kind")); kind != "" {
		if kind != "expense" && kind != "income" {
			writeErr(w, 400, "invalid_request", "kind 必须为 expense/income")
			return
		}
		query += ` AND kind=?`
		args = append(args, kind)
	}
	query += ` ORDER BY kind, sort, name, id`
	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []category{}
	for rows.Next() {
		var c category
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.Emoji, &c.Sort, &c.Active); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
		Sort  *int   `json:"sort"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind != "income" && in.Kind != "expense" {
		writeErr(w, 400, "invalid_request", "kind 必须为 expense/income")
		return
	}
	if !validCategoryName(in.Name) {
		writeErr(w, 400, "invalid_request", "分类名称须为 1 至 50 个字符")
		return
	}
	if !validCategoryEmoji(in.Emoji) {
		writeErr(w, 400, "invalid_request", "分类表情格式不正确")
		return
	}
	sort := 0
	if in.Sort != nil {
		sort = *in.Sort
	}
	id := newID("cat")
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO categories(id,household_id,kind,name,emoji,sort) VALUES(?,?,?,?,?,?)`,
		id, p.HouseholdID, in.Kind, in.Name, in.Emoji, sort); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, category{ID: id, Kind: in.Kind, Name: in.Name, Emoji: in.Emoji, Sort: sort, Active: 1})
}

func (a *API) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name   *string `json:"name"`
		Emoji  *string `json:"emoji"`
		Sort   *int    `json:"sort"`
		Active *bool   `json:"active"`
	}
	if err := decode(r, &in); err != nil || (in.Name == nil && in.Emoji == nil && in.Sort == nil && in.Active == nil) {
		writeErr(w, 400, "invalid_request", "请提供要更新的分类字段")
		return
	}
	if in.Name != nil {
		*in.Name = strings.TrimSpace(*in.Name)
		if !validCategoryName(*in.Name) {
			writeErr(w, 400, "invalid_request", "分类名称须为 1 至 50 个字符")
			return
		}
	}
	if in.Emoji != nil && !validCategoryEmoji(*in.Emoji) {
		writeErr(w, 400, "invalid_request", "分类表情格式不正确")
		return
	}
	var active any
	if in.Active != nil {
		if *in.Active {
			active = 1
		} else {
			active = 0
		}
	}
	id := r.PathValue("id")
	result, err := a.DB.ExecContext(r.Context(),
		`UPDATE categories SET name=COALESCE(?,name), emoji=COALESCE(?,emoji), sort=COALESCE(?,sort), active=COALESCE(?,active)
		 WHERE id=? AND household_id=? AND active>=0`,
		in.Name, in.Emoji, in.Sort, active, id, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		internalErr(w, err)
		return
	}
	if affected == 0 {
		if !a.categoryExists(r, p.HouseholdID, id) {
			writeErr(w, 404, "not_found", "分类不存在")
			return
		}
	}
	writeJSON(w, 200, map[string]string{"id": id})
}

// handleDeleteCategory is a SOFT delete: active=-1 keeps every historical
// journal reference and transaction row intact. Records are never physically
// removed, so past statistics can still resolve the category name.
func (a *API) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	ctx := r.Context()
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE categories SET active=-1 WHERE id=? AND household_id=? AND active>=0`, id, p.HouseholdID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			var existing string
			return tx.QueryRowContext(ctx,
				`SELECT id FROM categories WHERE id=? AND household_id=? AND active=-1`, id, p.HouseholdID).Scan(&existing)
		}
		return audit(ctx, tx, p, "delete_category", "category", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分类不存在")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}

func (a *API) categoryExists(r *http.Request, householdID, id string) bool {
	var n int
	return a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM categories WHERE id=? AND household_id=? AND active>=0`, id, householdID).Scan(&n) == nil && n == 1
}

// loadCategoryKind returns the kind of an ACTIVE household-scoped category.
func loadCategoryKind(ctx context.Context, q db.Queryer, householdID, id string) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx,
		`SELECT kind FROM categories WHERE id=? AND household_id=? AND active=1`, id, householdID).Scan(&kind)
	return kind, err
}
