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
	"tinynest/api/internal/money"
)

var errDebtOpeningUnavailable = errors.New("找不到最初记下的本金")
var errDebtPaymentsUnresolved = errors.New("请先撤销这份合同的付款。旧版本在其他日期撤销的付款需要单独核对")

const debtHasEffectivePayments = `EXISTS(
	SELECT 1 FROM debt_schedule_items s
	LEFT JOIN debt_payments pay ON pay.schedule_item_id=s.id
	WHERE s.contract_id=c.id AND (s.paid_minor<>0 OR (pay.id IS NOT NULL AND (
		pay.reversed=0 OR NOT EXISTS(
			SELECT 1 FROM journal_entries r
			WHERE r.household_id=c.household_id AND r.reversal_of=pay.entry_id AND r.occurred_on=pay.paid_on
		)
	)))
)`

func (a *API) handleReverseDebtOpening(w http.ResponseWriter, r *http.Request) {
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
		status, principal, err := a.lockDebtContract(r, tx, p.HouseholdID, id)
		if err != nil {
			return err
		}
		if status == "deleted" {
			return sql.ErrNoRows
		}
		if principal <= 0 {
			return errDebtOpeningUnavailable
		}
		var entryID string
		err = tx.QueryRowContext(ctx, `
			SELECT CASE WHEN COUNT(*)=1 THEN MIN(id) ELSE '' END FROM journal_entries
			WHERE household_id=? AND biz_type=? AND biz_id=?`,
			p.HouseholdID, ledger.BizOpeningLiability, id).Scan(&entryID)
		if err != nil {
			return err
		}
		if entryID == "" {
			return errDebtOpeningUnavailable
		}
		if status == "cancelled" {
			return tx.QueryRowContext(ctx, `
				SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		var blocked bool
		if err := tx.QueryRowContext(ctx, `SELECT `+debtHasEffectivePayments+`
			FROM debt_contracts c WHERE c.household_id=? AND c.id=?`, p.HouseholdID, id).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return errDebtPaymentsUnresolved
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		if original.Status != ledger.StatusPosted || original.Delta(ledger.LoanSubject(id)) != -money.Minor(principal) {
			return errDebtOpeningUnavailable
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
		if err := cancelDebtPeriods(r, tx, p.HouseholdID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE debt_contracts SET status='cancelled',remaining_principal_minor=0 WHERE household_id=? AND id=?`,
			p.HouseholdID, id); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_debt_opening", "debt_contract", id, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "负债合同不存在")
	case errors.Is(err, errDebtPaymentsUnresolved):
		writeErr(w, 409, "debt_in_use", err.Error())
	case errors.Is(err, errDebtOpeningUnavailable):
		writeErr(w, 409, "debt_opening_unavailable", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "cancelled", "reversal_entry_id": reversalID})
	}
}
