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

var errCreditPaymentInconsistent = errors.New("还款记录和账单金额不一致")

func (a *API) handleReverseStatementPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	statementID, paymentID := r.PathValue("id"), r.PathValue("paymentId")
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
		query := `SELECT cycle_end_on,due_on,system_due_minor,paid_minor
			FROM credit_statements WHERE household_id=? AND id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		var cycleEnd, due string
		var systemDue, paid int64
		if err := tx.QueryRowContext(ctx, query, p.HouseholdID, statementID).Scan(&cycleEnd, &due, &systemDue, &paid); err != nil {
			return err
		}
		var entryID string
		var amount int64
		if err := tx.QueryRowContext(ctx, `
			SELECT entry_id,amount_minor FROM credit_payments WHERE household_id=? AND statement_id=? AND id=?`,
			p.HouseholdID, statementID, paymentID).Scan(&entryID, &amount); err != nil {
			return err
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.Status == ledger.StatusReversed {
			return tx.QueryRowContext(ctx, `
				SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		if original.BizType != ledger.BizCreditRepayment || original.BizID != paymentID || amount <= 0 || amount > paid {
			return errCreditPaymentInconsistent
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
			p.HouseholdID, entryID); err != nil {
			return err
		}
		newPaid := paid - amount
		status := deriveStatementStatus(cycleEnd, due, a.businessDate(), systemDue, newPaid)
		if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET paid_minor=?,status=? WHERE household_id=? AND id=?`,
			newPaid, status, p.HouseholdID, statementID); err != nil {
			return err
		}
		if err := a.restoreStatementReminder(ctx, tx, p.HouseholdID, statementID); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_credit_payment", "credit_payment", paymentID, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "账单或还款记录不存在")
	case errors.Is(err, errCreditPaymentInconsistent):
		writeErr(w, 409, "credit_payment_inconsistent", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": paymentID, "status": "reversed", "reversal_entry_id": reversalID})
	}
}
