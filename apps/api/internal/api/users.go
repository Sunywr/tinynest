package api

import (
	"database/sql"
	"net/http"
)

func (a *API) adminRoute(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	a.route(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
		p, _ := principal(r)
		if p.Role != "admin" {
			writeErr(w, 403, "forbidden", "仅管理员可执行此操作")
			return
		}
		handler(w, r)
	})
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id,username,role,status FROM users
		WHERE household_id=? ORDER BY username`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, username, role, status string
		if err := rows.Scan(&id, &username, &role, &status); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, map[string]string{"id": id, "username": username, "role": role, "status": status})
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) handleDisableUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var matched bool
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var found string
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE id=? AND household_id=? AND role='member'`,
			id, p.HouseholdID).Scan(&found)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		matched = true
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET status='disabled' WHERE id=? AND household_id=? AND role='member'`, id, p.HouseholdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, a.nowUTC(), id); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "disable_user", "user", id, "{}")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	if !matched {
		writeErr(w, 404, "not_found", "成员账号不存在")
		return
	}
	writeJSON(w, 200, map[string]string{"id": id, "status": "disabled"})
}
