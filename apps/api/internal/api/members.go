package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (a *API) handleEnsureCurrentMember(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	var member householdMember
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		// Lock the authenticated user row before looking up the binding. This
		// serializes simultaneous first entries even across API processes.
		if _, err := tx.ExecContext(ctx, `UPDATE users SET id=id WHERE id=? AND household_id=?`, p.UserID, p.HouseholdID); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, `
			SELECT m.id, m.name, m.status FROM household_user_members b
			JOIN household_members m ON m.id=b.member_id
			WHERE b.user_id=? AND m.household_id=?`, p.UserID, p.HouseholdID).
			Scan(&member.ID, &member.Name, &member.Status)
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		member = householdMember{ID: newID("mem"), Name: p.Username, Status: "active"}
		if _, err := tx.ExecContext(ctx, `INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`,
			member.ID, p.HouseholdID, member.Name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO household_user_members(user_id,member_id) VALUES(?,?)`,
			p.UserID, member.ID); err != nil {
			return err
		}
		return audit(ctx, tx, p, "bind_current_member", "household_member", member.ID, "{}")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, member)
}

type householdMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (a *API) handleListMembers(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, name, status FROM household_members
		WHERE household_id=? AND status<>'deleted' ORDER BY name, id`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()

	items := []householdMember{}
	for rows.Next() {
		var member householdMember
		if err := rows.Scan(&member.ID, &member.Name, &member.Status); err != nil {
			internalErr(w, err)
			return
		}
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}

func (a *API) lockMember(r *http.Request, tx *sql.Tx, householdID, id string) (string, error) {
	query := `SELECT status FROM household_members WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var status string
	err := tx.QueryRowContext(r.Context(), query, householdID, id).Scan(&status)
	return status, err
}

var errMemberInUse = errors.New("成员档案已关联登录、邀请或账务记录")

func (a *API) handleDeleteMember(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	ctx := r.Context()
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		status, err := a.lockMember(r, tx, p.HouseholdID, id)
		if err != nil || status == "deleted" {
			return err
		}
		var used bool
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM household_user_members b JOIN users u ON u.id=b.user_id
				WHERE u.household_id=? AND b.member_id=?)
			OR EXISTS(SELECT 1 FROM household_invites WHERE household_id=? AND member_id=?)
			OR EXISTS(SELECT 1 FROM accounts WHERE household_id=? AND owner_member_id=?)
			OR EXISTS(SELECT 1 FROM transactions WHERE household_id=? AND member_id=?)
			OR EXISTS(SELECT 1 FROM debt_contracts WHERE household_id=? AND member_id=?)
			OR EXISTS(SELECT 1 FROM credit_instruments WHERE household_id=? AND member_id=?)`,
			p.HouseholdID, id, p.HouseholdID, id, p.HouseholdID, id,
			p.HouseholdID, id, p.HouseholdID, id, p.HouseholdID, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return errMemberInUse
		}
		if _, err := tx.ExecContext(ctx, `UPDATE household_members SET status='deleted' WHERE household_id=? AND id=?`,
			p.HouseholdID, id); err != nil {
			return err
		}
		return audit(ctx, tx, p, "delete_household_member", "household_member", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "成员档案不存在")
	case errors.Is(err, errMemberInUse):
		writeErr(w, 409, "member_in_use", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}

func (a *API) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		writeErr(w, 400, "invalid_request", "成员名称须为 1 至 100 个字符")
		return
	}

	member := householdMember{ID: newID("mem"), Name: name, Status: "active"}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`,
			member.ID, p.HouseholdID, member.Name); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "create", "household_member", member.ID, "{}")
	})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, member)
}
