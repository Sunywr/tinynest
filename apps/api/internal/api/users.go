package api

import (
	"database/sql"
	"net/http"
	"strings"

	"tinynest/api/internal/auth"
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

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len(in.Username) > 100 || len(in.Password) < 12 || len(in.Password) > 256 {
		writeErr(w, 400, "invalid_request", "用户名必填，密码须为 12 至 256 字节")
		return
	}
	var count int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users WHERE username=?`, in.Username).Scan(&count); err != nil {
		internalErr(w, err)
		return
	}
	if count != 0 {
		writeErr(w, 409, "username_unavailable", "用户名不可用")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(w, err)
		return
	}
	id := newID("usr")
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO users(id,household_id,username,password_hash,role)
			VALUES(?,?,?,?,'member')`, id, p.HouseholdID, in.Username, hash); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "create_user", "user", id, "{}")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "username": in.Username, "role": "member"})
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
