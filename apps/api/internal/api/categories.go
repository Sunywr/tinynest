package api

import (
	"database/sql"
	"errors"
	"net/http"
)

func (a *API) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	a.deleteClassification(w, r, "category", "分类不存在",
		`UPDATE categories SET active=-1 WHERE id=? AND household_id=? AND active>=0`,
		`SELECT id FROM categories WHERE id=? AND household_id=? AND active=-1`)
}

// Tombstones preserve existing journal and transaction references.
func (a *API) deleteClassification(w http.ResponseWriter, r *http.Request, entity, missing, update, lookup string) {
	p, _ := principal(r)
	id := r.PathValue("id")
	ctx := r.Context()
	hasChildren := errors.New("请先删除此分类下的子分类")
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, update, id, p.HouseholdID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			var existing string
			return tx.QueryRowContext(ctx, lookup, id, p.HouseholdID).Scan(&existing)
		}
		if entity == "category" {
			var children int
			err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE parent_id=? AND household_id=? AND active>=0`,
				id, p.HouseholdID).Scan(&children)
			if err != nil {
				return err
			}
			if children > 0 {
				return hasChildren
			}
		}
		return audit(ctx, tx, p, "delete_"+entity, entity, id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", missing)
	case errors.Is(err, hasChildren):
		writeErr(w, 409, "category_has_children", hasChildren.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}
