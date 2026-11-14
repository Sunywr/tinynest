package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"tinynest/api/internal/db"
)

var errInvalidAccountOwner = errors.New("归属成员不存在或已停用")

func validateAccountOwner(r *http.Request, tx *sql.Tx, householdID string, owner *string) error {
	if owner == nil || *owner == "" {
		return nil
	}
	var id string
	err := tx.QueryRowContext(r.Context(), `SELECT id FROM household_members
		WHERE id=? AND household_id=? AND status='active'`, *owner, householdID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errInvalidAccountOwner
	}
	return err
}

func (a *API) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var name, kind, code, status string
	var subtype, owner, note *string
	err := a.DB.QueryRowContext(r.Context(), `SELECT name,kind,ledger_code,status,subtype,owner_member_id,note
		FROM accounts WHERE id=? AND household_id=?`, id, p.HouseholdID).
		Scan(&name, &kind, &code, &status, &subtype, &owner, &note)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "账户不存在")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	balance, err := db.Balance(r.Context(), a.DB, p.HouseholdID, code)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "name": name, "kind": kind, "ledger_code": code, "status": status,
		"subtype": subtype, "owner_member_id": owner, "note": note, "balance_minor": balance,
	})
}

func (a *API) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var in struct {
		Name    *string `json:"name"`
		Subtype *string `json:"subtype"`
		Owner   *string `json:"owner_member_id"`
		Note    *string `json:"note"`
	}
	if err := decode(r, &in); err != nil || (in.Name == nil && in.Subtype == nil && in.Owner == nil && in.Note == nil) {
		writeErr(w, 400, "invalid_request", "请提供要修改的账户资料")
		return
	}
	if in.Name != nil {
		*in.Name = strings.TrimSpace(*in.Name)
		if *in.Name == "" || len([]rune(*in.Name)) > 100 {
			writeErr(w, 400, "invalid_request", "账户名称须为 1 至 100 个字符")
			return
		}
	}
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var code string
		if err := tx.QueryRowContext(r.Context(), `SELECT ledger_code FROM accounts WHERE id=? AND household_id=?`,
			id, p.HouseholdID).Scan(&code); err != nil {
			return err
		}
		if err := validateAccountOwner(r, tx, p.HouseholdID, in.Owner); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE accounts SET
			name=COALESCE(?,name),
			subtype=CASE WHEN ? IS NULL THEN subtype ELSE NULLIF(?,'') END,
			owner_member_id=CASE WHEN ? IS NULL THEN owner_member_id ELSE NULLIF(?,'') END,
			note=CASE WHEN ? IS NULL THEN note ELSE NULLIF(?,'') END
			WHERE id=? AND household_id=?`,
			in.Name, in.Subtype, in.Subtype, in.Owner, in.Owner, in.Note, in.Note, id, p.HouseholdID); err != nil {
			return err
		}
		if in.Name != nil {
			if _, err := tx.ExecContext(r.Context(), `UPDATE ledger_accounts SET name=? WHERE code=? AND household_id=?`,
				*in.Name, code, p.HouseholdID); err != nil {
				return err
			}
		}
		return audit(r.Context(), tx, p, "update_account", "account", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "账户不存在")
	case errors.Is(err, errInvalidAccountOwner):
		writeErr(w, 400, "invalid_request", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id})
	}
}
