package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
)

var errInstallmentInUse = errors.New("存在未撤销的付款、退款记录，或找不到原分期的记账记录")

func (a *API) lockInstallment(r *http.Request, tx *sql.Tx, householdID, id string) (string, sql.NullString, error) {
	query := `SELECT status,source_transaction_id FROM installment_contracts WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var status string
	var source sql.NullString
	err := tx.QueryRowContext(r.Context(), query, householdID, id).Scan(&status, &source)
	return status, source, err
}

func (a *API) requireUncancelledInstallment(r *http.Request, tx *sql.Tx, householdID, id string) error {
	status, _, err := a.lockInstallment(r, tx, householdID, id)
	if errors.Is(err, sql.ErrNoRows) || status == "cancelled" {
		return errors.New("分期合同不存在或已撤销")
	}
	return err
}

func (a *API) handleReverseInstallment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 200 {
		writeErr(w, 400, "invalid_request", "撤销原因须为 1 至 200 个字符")
		return
	}
	var reversalID string
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		status, source, err := a.lockInstallment(r, tx, p.HouseholdID, id)
		if err != nil {
			return err
		}
		entryID, err := a.installmentOpeningEntry(r, tx, p.HouseholdID, id, source)
		if err != nil {
			return err
		}
		if status == "cancelled" {
			return tx.QueryRowContext(ctx, `
				SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		var used bool
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM installment_schedule_items s
				JOIN installment_contracts c ON c.id=s.contract_id
				LEFT JOIN installment_payments pay ON pay.schedule_item_id=s.id
				WHERE c.household_id=? AND c.id=? AND (s.paid_minor<>0 OR (pay.id IS NOT NULL AND pay.reversed=0)))
			OR EXISTS(SELECT 1 `+refundJournalJoin+` WHERE r.household_id=? AND r.transaction_id=? AND e.status='posted')`,
			p.HouseholdID, id, p.HouseholdID, source.String).Scan(&used); err != nil {
			return err
		}
		if used {
			return errInstallmentInUse
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.Status != ledger.StatusPosted {
			return errInstallmentInUse
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`, p.HouseholdID, entryID); err != nil {
			return err
		}
		if source.Valid {
			if original.BizType != ledger.BizInstallmentNew || original.BizID != source.String {
				return errInstallmentInUse
			}
			if _, err := tx.ExecContext(ctx, `UPDATE transactions SET status='reversed' WHERE household_id=? AND id=?`, p.HouseholdID, source.String); err != nil {
				return err
			}
		} else if err := a.restoreConvertedStatement(r, tx, p.HouseholdID, id, original); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE installment_schedule_items SET status='cancelled' WHERE contract_id IN (
				SELECT id FROM installment_contracts WHERE household_id=? AND id=?
			)`, p.HouseholdID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM schedule_due_items WHERE household_id=? AND source_type='installment' AND status='pending'
			AND source_id IN (SELECT id FROM installment_schedule_items WHERE contract_id=?)`, p.HouseholdID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE installment_contracts SET status='cancelled' WHERE household_id=? AND id=?`, p.HouseholdID, id); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_installment", "installment_contract", id, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分期合同不存在")
	case errors.Is(err, errInstallmentInUse):
		writeErr(w, 409, "installment_in_use", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "cancelled", "reversal_entry_id": reversalID})
	}
}

func (a *API) installmentOpeningEntry(r *http.Request, tx *sql.Tx, householdID, id string, source sql.NullString) (string, error) {
	var entryID string
	if source.Valid {
		query := `SELECT entry_id FROM transactions WHERE household_id=? AND id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		err := tx.QueryRowContext(r.Context(), query, householdID, source.String).Scan(&entryID)
		return entryID, err
	}
	err := tx.QueryRowContext(r.Context(), `
		SELECT MIN(id) FROM journal_entries WHERE household_id=? AND biz_type=? AND biz_id=? HAVING COUNT(*)=1`,
		householdID, ledger.BizConvertInstall, id).Scan(&entryID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errInstallmentInUse
	}
	return entryID, err
}
