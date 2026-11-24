package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (a *API) handleCurrentHousehold(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var name, currency, timezone string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT name,currency,timezone FROM households WHERE id=?`, p.HouseholdID).Scan(&name, &currency, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "家庭不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": p.HouseholdID, "name": name, "currency": currency, "timezone": timezone})
}
func (a *API) handleUpdateHousehold(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct{ ID, Name string }
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	err := decode(r, &in)
	name := strings.TrimSpace(in.Name)
	if err != nil || name == "" || utf8.RuneCountInString(name) > 100 {
		writeErr(w, 400, "invalid_request", "请输入家庭名称（最多 100 字）")
		return
	}
	if in.ID != p.HouseholdID {
		writeErr(w, 409, "conflict", "当前家庭已变化，请刷新页面")
		return
	}
	ctx := r.Context()
	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.lockHouseholdActor(r, tx, p); err != nil {
			return err
		}
		var current string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM households WHERE id=?`, p.HouseholdID).Scan(&current); err != nil {
			return err
		}
		if current == name {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE households SET name=? WHERE id=?`, name, p.HouseholdID); err != nil {
			return err
		}
		return audit(ctx, tx, p, "update", "household", p.HouseholdID, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 403, "forbidden", "仅家庭管理员可以修改家庭设置")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": p.HouseholdID, "name": name})
	}
}
func (a *API) handleArchiveHousehold(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	if r.URL.Query().Get("id") != p.HouseholdID {
		writeErr(w, 409, "conflict", "当前家庭已变化，请刷新页面")
		return
	}
	ctx := r.Context()
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.lockHouseholdActor(r, tx, p); err != nil {
			return err
		}
		insert := `INSERT INTO households(id,name) VALUES(?,?) ON CONFLICT(id) DO NOTHING`
		if a.DB.IsMySQL() {
			insert = `INSERT INTO households(id,name) VALUES(?,?) ON DUPLICATE KEY UPDATE id=id`
		}
		if _, err := tx.ExecContext(ctx, insert, onboardingHousehold, "账号开通"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE household_invites SET decided_at=?
			WHERE household_id=? AND decided_at IS NULL`, a.nowUTC(), p.HouseholdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM household_user_members
			WHERE user_id IN (SELECT id FROM users WHERE household_id=?)`, p.HouseholdID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET household_id=?,role='member'
			WHERE household_id=?`, onboardingHousehold, p.HouseholdID); err != nil {
			return err
		}
		return audit(ctx, tx, p, "archive", "household", p.HouseholdID, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 403, "forbidden", "仅家庭管理员可以删除家庭")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"status": "archived"})
	}
}

func (a *API) lockHouseholdActor(r *http.Request, tx *sql.Tx, p Principal) error {
	ctx := r.Context()
	lock := ""
	if a.DB.IsMySQL() {
		lock = " FOR UPDATE"
	}
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE id=?`+lock, p.HouseholdID).Scan(&id); err != nil {
		return err
	}
	return tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? AND household_id=? AND role=? AND status='active'`+lock,
		p.UserID, p.HouseholdID, p.Role).Scan(&id)
}
