package api

import (
	"database/sql"
	"errors"
	"net/http"
)

var errDebtHasLedgerHistory = errors.New("已有本金或付款记录的固定负债不能删除")

func (a *API) lockDebtContract(r *http.Request, tx *sql.Tx, householdID, id string) (string, int64, error) {
	query := `SELECT status,principal_minor FROM debt_contracts WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var status string
	var principal int64
	err := tx.QueryRowContext(r.Context(), query, householdID, id).Scan(&status, &principal)
	return status, principal, err
}

func (a *API) handleDeleteDebt(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	id := r.PathValue("id")
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		status, principal, err := a.lockDebtContract(r, tx, p.HouseholdID, id)
		if err != nil || status == "deleted" {
			return err
		}
		if principal != 0 {
			return errDebtHasLedgerHistory
		}
		if err := checkDebtPaymentHistory(r, tx, p.HouseholdID, id); err != nil {
			return err
		}
		if err := cancelDebtPeriods(r, tx, p.HouseholdID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE debt_contracts SET status='deleted' WHERE household_id=? AND id=?`,
			p.HouseholdID, id); err != nil {
			return err
		}
		return audit(ctx, tx, p, "delete_debt", "debt_contract", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "负债合同不存在")
	case errors.Is(err, errDebtHasLedgerHistory):
		writeErr(w, 409, "debt_in_use", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}

func checkDebtPaymentHistory(r *http.Request, tx *sql.Tx, householdID, id string) error {
	var used bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(
		SELECT 1 FROM debt_schedule_items s
		JOIN debt_contracts c ON c.id=s.contract_id
		LEFT JOIN debt_payments pay ON pay.schedule_item_id=s.id
		WHERE c.household_id=? AND c.id=? AND (s.paid_minor<>0 OR pay.id IS NOT NULL)
	)`, householdID, id).Scan(&used); err != nil {
		return err
	}
	if used {
		return errDebtHasLedgerHistory
	}
	return nil
}

func cancelDebtPeriods(r *http.Request, tx *sql.Tx, householdID, id string) error {
	ctx := r.Context()
	if _, err := tx.ExecContext(ctx, `
		UPDATE debt_schedule_items SET status='cancelled' WHERE contract_id IN (
			SELECT id FROM debt_contracts WHERE household_id=? AND id=?
		)`, householdID, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		DELETE FROM schedule_due_items WHERE household_id=? AND source_type='debt' AND status='pending'
		AND source_id IN (SELECT id FROM debt_schedule_items WHERE contract_id=?)`, householdID, id)
	return err
}
