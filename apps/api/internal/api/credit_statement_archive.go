package api

import (
	"database/sql"
	"errors"
	"net/http"
)

var errStatementNotSettled = errors.New("只有未还金额为零的账单可以归档")

func (a *API) handleArchiveStatement(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var in struct {
		Archived *bool `json:"archived"`
	}
	if err := decode(r, &in); err != nil || in.Archived == nil {
		writeErr(w, 400, "invalid_request", "请指定归档状态")
		return
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		query := `SELECT status,cycle_end_on,due_on,system_due_minor,paid_minor
			FROM credit_statements WHERE household_id=? AND id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		var status, cycleEnd, due string
		var systemDue, paid int64
		if err := tx.QueryRowContext(ctx, query, p.HouseholdID, id).Scan(&status, &cycleEnd, &due, &systemDue, &paid); err != nil {
			return err
		}
		if *in.Archived && systemDue != paid {
			return errStatementNotSettled
		}
		if *in.Archived == (status == "archived") {
			return nil
		}
		action := "restore_credit_statement"
		newStatus := deriveStatementStatus(cycleEnd, due, a.businessDate(), systemDue, paid)
		if *in.Archived {
			action, newStatus = "archive_credit_statement", "archived"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET status=? WHERE household_id=? AND id=?`,
			newStatus, p.HouseholdID, id); err != nil {
			return err
		}
		if *in.Archived {
			if err := a.refreshPendingStatementAmount(ctx, tx, p.HouseholdID, id); err != nil {
				return err
			}
		}
		return audit(ctx, tx, p, action, "credit_statement", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "账单不存在")
	case errors.Is(err, errStatementNotSettled):
		writeErr(w, 409, "statement_not_settled", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]any{"id": id, "archived": *in.Archived})
	}
}
