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

var errInstallmentPaymentConflict = errors.New("付款记录与期次不一致")
var errInstallmentPaymentRepaid = errors.New("原信用账单可冲减金额不足，请先核对相关的账单还款，录错的还款可以撤销")

func (a *API) handleReverseInstallmentPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	contractID, paymentID := r.PathValue("id"), r.PathValue("paymentId")
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
		contractStatus, _, err := a.lockInstallment(r, tx, p.HouseholdID, contractID)
		if err != nil {
			return err
		}
		var entryID, channel, channelID, periodID, due, status, name string
		var amount, paid, total int64
		var number, reversed int
		if err := tx.QueryRowContext(ctx, `
			SELECT pay.entry_id,pay.channel,pay.channel_id,pay.amount_minor,pay.reversed,
			       s.id,s.due_on,s.status,s.paid_minor,s.total_minor,s.installment_no,c.name
			FROM installment_payments pay
			JOIN installment_schedule_items s ON s.id=pay.schedule_item_id
			JOIN installment_contracts c ON c.id=s.contract_id
			WHERE c.household_id=? AND c.id=? AND pay.id=?`,
			p.HouseholdID, contractID, paymentID).Scan(&entryID, &channel, &channelID, &amount, &reversed,
			&periodID, &due, &status, &paid, &total, &number, &name); err != nil {
			return err
		}
		if reversed == 1 {
			return tx.QueryRowContext(ctx, `SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		if contractStatus == "cancelled" || status == "cancelled" || amount <= 0 || paid < amount || total < paid {
			return errInstallmentPaymentConflict
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.BizType != ledger.BizInstallmentPay || original.BizID != paymentID || original.Status != ledger.StatusPosted {
			return errInstallmentPaymentConflict
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if channel == "credit" {
			if err := a.reversePeriodCreditCharge(r, tx, p.HouseholdID, channelID, "installment_payment", paymentID, reversalID, amount); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
			p.HouseholdID, entryID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE installment_payments SET reversed=1 WHERE id=? AND schedule_item_id=?`,
			paymentID, periodID); err != nil {
			return err
		}
		newPaid := paid - amount
		if _, err := tx.ExecContext(ctx, `UPDATE installment_schedule_items SET paid_minor=?,status=? WHERE id=? AND contract_id=?`,
			newPaid, derivePeriodStatus(due, a.businessDate(), total, newPaid), periodID, contractID); err != nil {
			return err
		}
		if err := a.restoreDueReminder(ctx, tx, p.HouseholdID, dueReminder{
			kind: "installment", id: periodID, due: due, label: name + " 第" + itoa(number) + "期", remaining: total - newPaid,
		}); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_installment_payment", "installment_payment", paymentID, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分期合同或付款记录不存在")
	case errors.Is(err, errInstallmentPaymentConflict), errors.Is(err, errInstallmentPaymentRepaid):
		writeErr(w, 409, "installment_payment_conflict", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": paymentID, "status": "reversed", "reversal_entry_id": reversalID})
	}
}

func (a *API) reversePeriodCreditCharge(r *http.Request, tx *sql.Tx, householdID, instrumentID, sourceType, paymentID, reversalID string, amount int64) error {
	ctx := r.Context()
	query := `SELECT id FROM credit_instruments WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var lockedID string
	if err := tx.QueryRowContext(ctx, query, householdID, instrumentID).Scan(&lockedID); err != nil {
		return err
	}
	var statementID, originalDate string
	err := tx.QueryRowContext(ctx, `
		SELECT MIN(s.id),MIN(i.occurred_on)
		FROM credit_statement_items i JOIN credit_statements s ON s.id=i.statement_id
		WHERE s.household_id=? AND s.instrument_id=? AND i.source_type=?
		  AND i.source_id=? AND i.amount_minor=? HAVING COUNT(*)=1`,
		householdID, instrumentID, sourceType, paymentID, amount).Scan(&statementID, &originalDate)
	if errors.Is(err, sql.ErrNoRows) {
		return errInstallmentPaymentConflict
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE credit_statements SET system_due_minor=system_due_minor-?
		WHERE household_id=? AND id=? AND system_due_minor-paid_minor>=?`,
		amount, householdID, statementID, amount)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errInstallmentPaymentRepaid
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
		VALUES(?,?,?,?,?)`, statementID, sourceType+"_reversal", reversalID, -amount, originalDate); err != nil {
		return err
	}
	return a.restoreStatementReminder(ctx, tx, householdID, statementID)
}
