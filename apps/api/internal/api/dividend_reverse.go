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

func (a *API) handleReverseDividend(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	ctx := r.Context()
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
		query := `
			SELECT d.entry_id,d.asset_id,d.net_minor,e.status FROM investment_dividends d
			JOIN journal_entries e ON e.id=d.entry_id AND e.household_id=d.household_id
			WHERE d.household_id=? AND d.id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		var entryID, assetID, status string
		var net int64
		if err := tx.QueryRowContext(ctx, query, p.HouseholdID, id).Scan(&entryID, &assetID, &net, &status); err != nil {
			return err
		}
		if status == ledger.StatusReversed {
			return tx.QueryRowContext(ctx, `
				SELECT id FROM journal_entries WHERE household_id=? AND reversal_of=?`,
				p.HouseholdID, entryID).Scan(&reversalID)
		}
		original, err := loadEntry(tx, r, entryID)
		if err != nil {
			return err
		}
		reversalID = newID("je")
		reversal, err := original.Reverse(reversalID, original.OccurredOn)
		if err != nil {
			return err
		}
		if err := db.SaveEntry(ctx, tx, reversal, p.HouseholdID, "", p.Username); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE journal_entries SET status='reversed' WHERE household_id=? AND id=?`,
			p.HouseholdID, entryID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE investment_positions SET net_dividend_minor=net_dividend_minor-?
			WHERE asset_id IN (SELECT id FROM investment_assets WHERE household_id=? AND id=?)`,
			net, p.HouseholdID, assetID); err != nil {
			return err
		}
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "reverse_dividend", "investment_dividend", id, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "分红记录不存在")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "reversed", "reversal_entry_id": reversalID})
	}
}
