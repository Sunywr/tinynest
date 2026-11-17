package api

import (
	"database/sql"
	"errors"
	"net/http"
)

var errCreditInstrumentUsed = errors.New("信用工具已有账务记录，可停用以保留历史")

func (a *API) handleDeleteInstrument(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	id := r.PathValue("id")
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		query := `SELECT status,ledger_code FROM credit_instruments WHERE id=? AND household_id=?`
		if a.DB.IsMySQL() {
			query += " FOR UPDATE"
		}
		var status, code string
		if err := tx.QueryRowContext(ctx, query, id, p.HouseholdID).Scan(&status, &code); err != nil {
			return err
		}
		if status == "deleted" {
			return nil
		}
		var used bool
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM credit_statements WHERE household_id=? AND instrument_id=?)
			OR EXISTS(SELECT 1 FROM transactions WHERE household_id=? AND credit_instrument_id=?)
			OR EXISTS(SELECT 1 FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
				WHERE e.household_id=? AND l.account_code=?)`,
			p.HouseholdID, id, p.HouseholdID, id, p.HouseholdID, code).Scan(&used); err != nil {
			return err
		}
		if used {
			return errCreditInstrumentUsed
		}
		if _, err := tx.ExecContext(ctx, `UPDATE credit_instruments SET status='deleted' WHERE id=? AND household_id=?`,
			id, p.HouseholdID); err != nil {
			return err
		}
		return audit(ctx, tx, p, "delete_credit_instrument", "credit_instrument", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "信用工具不存在")
	case errors.Is(err, errCreditInstrumentUsed):
		writeErr(w, 409, "credit_instrument_in_use", err.Error())
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}
