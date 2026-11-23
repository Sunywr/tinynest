package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// errLastAdmin blocks demoting the only active admin: without one, nobody could
// invite members, approve joins or manage accounts any more.
var errLastAdmin = errors.New("last_admin")

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
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT u.id,u.username,u.role,u.status,u.email,u.avatar,u.birthday,u.payday,m.name
		FROM users u
		LEFT JOIN household_user_members b ON b.user_id=u.id
		LEFT JOIN household_members m ON m.id=b.member_id AND m.household_id=u.household_id
		WHERE u.household_id=? ORDER BY u.username`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, username, role, status string
		var email, avatar, birthday, memberName sql.NullString
		var payday sql.NullInt64
		if err := rows.Scan(&id, &username, &role, &status, &email, &avatar, &birthday, &payday, &memberName); err != nil {
			internalErr(w, err)
			return
		}
		item := map[string]any{"id": id, "username": username, "role": role, "status": status}
		if memberName.Valid {
			item["member_name"] = memberName.String
		}
		if email.Valid {
			item["email"] = email.String
		}
		if avatar.Valid {
			item["avatar"] = avatar.String
		}
		if birthday.Valid {
			item["birthday"] = birthday.String
		}
		if payday.Valid {
			item["payday"] = payday.Int64
		}
		items = append(items, item)
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
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE id=? AND household_id=? AND role IN ('member','viewer')`,
			id, p.HouseholdID).Scan(&found)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		matched = true
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET status='disabled' WHERE id=? AND household_id=? AND role IN ('member','viewer')`, id, p.HouseholdID); err != nil {
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

func (a *API) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Role string `json:"role"`
	}
	if err := decode(r, &in); err != nil || (in.Role != "admin" && in.Role != "member" && in.Role != "viewer") {
		writeErr(w, 400, "invalid_request", "角色须为管理员、成员或只读")
		return
	}
	id := r.PathValue("id")
	// Admins manage other accounts only; self-demotion is refused so an admin
	// can never lock themselves out of household management.
	if id == p.UserID {
		writeErr(w, 403, "forbidden", "不能修改自己的角色")
		return
	}
	ctx := r.Context()
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var role string
		err := tx.QueryRowContext(ctx, `SELECT role FROM users
			WHERE id=? AND household_id=? AND status='active'`, id, p.HouseholdID).Scan(&role)
		if err != nil || role == in.Role {
			return err
		}
		if role == "admin" {
			// Serialize admin demotions per household so two admins cannot step
			// down concurrently and leave the household without one. The row lock
			// is taken before the count on MySQL; SQLite already has one writer.
			lock := `SELECT id FROM households WHERE id=?`
			if a.DB.IsMySQL() {
				lock += " FOR UPDATE"
			}
			var householdID string
			if err := tx.QueryRowContext(ctx, lock, p.HouseholdID).Scan(&householdID); err != nil {
				return err
			}
			var admins int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users
				WHERE household_id=? AND role='admin' AND status='active'`, p.HouseholdID).Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return errLastAdmin
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET role=?
			WHERE id=? AND household_id=? AND status='active'`, in.Role, id, p.HouseholdID); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"role": in.Role})
		return audit(ctx, tx, p, "update_user_role", "user", id, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "成员账号不存在或已停用")
	case errors.Is(err, errLastAdmin):
		writeErr(w, 409, "last_admin", "家庭至少需要一位管理员；请先将另一位成员设为管理员")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "role": in.Role})
	}
}
