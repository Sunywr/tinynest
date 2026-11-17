package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
)

const refundJournalJoin = `FROM refunds r JOIN journal_entries e ON e.id=r.entry_id AND e.household_id=r.household_id`

type installmentRefundHistoryRow struct {
	ID          string `json:"id"`
	RefundOn    string `json:"refund_on"`
	Destination string `json:"destination_type"`
	AmountMinor int64  `json:"amount_minor"`
	Status      string `json:"status"`
}

func listInstallmentRefunds(ctx context.Context, q db.Queryer, householdID, transactionID string) ([]installmentRefundHistoryRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id,r.refund_on,r.destination_type,r.amount_minor,e.status `+refundJournalJoin+`
		WHERE r.household_id=? AND r.transaction_id=? ORDER BY r.refund_on DESC,r.id DESC`, householdID, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []installmentRefundHistoryRow{}
	for rows.Next() {
		var item installmentRefundHistoryRow
		if err := rows.Scan(&item.ID, &item.RefundOn, &item.Destination, &item.AmountMinor, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func refundTotalForTransaction(ctx context.Context, q db.Queryer, householdID, transactionID string) (int64, error) {
	var total int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(r.amount_minor),0) `+refundJournalJoin+`
		WHERE r.household_id=? AND r.transaction_id=? AND e.status='posted'`, householdID, transactionID).Scan(&total)
	return total, err
}

var errRefundUsedInPlan = errors.New("付款计划已使用这笔退款，撤销后退款总额会低于已减免的金额")
var errRefundRecordUnavailable = errors.New("找不到可撤销的退款记账记录")

func (a *API) handleReverseInstallmentRefund(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	contractID, refundID := r.PathValue("id"), r.PathValue("refundId")
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
		status, source, err := a.lockInstallment(r, tx, p.HouseholdID, contractID)
		if err != nil {
			return err
		}
		if !source.Valid {
			return sql.ErrNoRows
		}
		// The original purchase also guards refunds from the regular ledger.
		if _, err := a.installmentOpeningEntry(r, tx, p.HouseholdID, contractID, source); err != nil {
			return err
		}
		var entryID, destination, destinationID, refundStatus string
		var amount int64
		if err := tx.QueryRowContext(ctx, `
			SELECT r.entry_id,r.destination_type,r.destination_id,r.amount_minor,e.status `+refundJournalJoin+`
			WHERE r.household_id=? AND r.transaction_id=? AND r.id=?`,
			p.HouseholdID, source.String, refundID).Scan(&entryID, &destination, &destinationID, &amount, &refundStatus); err != nil {
			return err
		}
		if refundStatus == ledger.StatusReversed {
			return tx.QueryRowContext(ctx, `SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		if status == "cancelled" || amount <= 0 || refundStatus != ledger.StatusPosted {
			return errRefundRecordUnavailable
		}
		refunded, err := refundTotalForTransaction(ctx, tx, p.HouseholdID, source.String)
		if err != nil {
			return err
		}
		var relieved int64
		if err := tx.QueryRowContext(ctx, `
			SELECT c.principal_minor+c.total_fee_minor-COALESCE(SUM(s.total_minor),0)
			FROM installment_contracts c
			LEFT JOIN installment_schedule_items s ON s.contract_id=c.id AND s.status<>'cancelled'
			WHERE c.household_id=? AND c.id=? GROUP BY c.id`, p.HouseholdID, contractID).Scan(&relieved); err != nil {
			return err
		}
		if refunded-amount < max(0, relieved) {
			return errRefundUsedInPlan
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.BizType != ledger.BizRefund || original.BizID != refundID {
			return errRefundRecordUnavailable
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if destination == "credit" {
			if err := a.restoreCreditRefund(r, tx, p.HouseholdID, destinationID, refundID, reversalID, amount); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
			p.HouseholdID, entryID); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_installment_refund", "refund", refundID, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分期或退款记录不存在")
	case errors.Is(err, errRefundUsedInPlan), errors.Is(err, errRefundRecordUnavailable):
		writeErr(w, 409, "refund_not_reversible", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": refundID, "status": "reversed", "reversal_entry_id": reversalID})
	}
}

func (a *API) restoreCreditRefund(r *http.Request, tx *sql.Tx, householdID, instrumentID, refundID, reversalID string, amount int64) error {
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
		WHERE s.household_id=? AND s.instrument_id=? AND i.source_type='refund'
		  AND i.source_id=? AND i.amount_minor=? HAVING COUNT(*)=1`,
		householdID, instrumentID, refundID, -amount).Scan(&statementID, &originalDate)
	if errors.Is(err, sql.ErrNoRows) {
		return errRefundRecordUnavailable
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credit_statements SET system_due_minor=system_due_minor+?
		WHERE household_id=? AND id=?`, amount, householdID, statementID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
		VALUES(?,'installment_refund_reversal',?,?,?)`, statementID, reversalID, amount, originalDate); err != nil {
		return err
	}
	return a.restoreStatementReminder(ctx, tx, householdID, statementID)
}
