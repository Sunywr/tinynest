package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"tinynest/api/internal/money"
)

func (a *API) handleBatchStatementPayment(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		From       string `json:"from"`
		To         string `json:"to"`
		PaidOn     string `json:"paid_on"`
		UseDueDate bool   `json:"use_due_date,omitempty"`
		Account    string `json:"from_account_id"`
		Items      []struct {
			ID     string `json:"id"`
			DueOn  string `json:"due_on,omitempty"`
			Amount int64  `json:"amount_minor"`
		} `json:"items"`
	}
	key := idempotencyKey(r)
	if decode(r, &in) != nil || !validDate(in.From) || !validDate(in.To) || in.To < in.From || (!in.UseDueDate && !validDate(in.PaidOn)) || (in.UseDueDate && in.PaidOn != "") || in.Account == "" || len(in.Items) == 0 || key == "" {
		writeErr(w, 400, "invalid_request", "请检查账单范围、付款账户和还款日期")
		return
	}
	payload, _ := json.Marshal(in)
	marker := fmt.Sprintf("batch:%x", sha256.Sum256(append(payload, []byte(key)...)))
	marker = scopedKey(p.HouseholdID, marker)
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRowContext(r.Context(), "SELECT id FROM credit_payments WHERE household_id=? AND idempotency_key=?", p.HouseholdID, marker).Scan(&existing)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for i, item := range in.Items {
			if item.Amount <= 0 {
				return errors.New("待还金额必须为正")
			}
			entryKey := ""
			if i == 0 {
				entryKey = marker
			}
			paidOn := in.PaidOn
			expectedDue := ""
			if in.UseDueDate {
				if !validDate(item.DueOn) {
					return errors.New("请重新预览账单截止日期")
				}
				expectedDue = item.DueOn
				paidOn = item.DueOn
			}
			if err := a.postStatementPayment(tx, r, p, item.ID, in.Account, paidOn, money.Minor(item.Amount), newID("cpay"), newID("je"), entryKey, in.From, in.To, expectedDue); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, 400, "invalid_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"count": len(in.Items)})
}
