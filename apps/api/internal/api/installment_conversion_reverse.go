package api

import (
	"database/sql"
	"errors"
	"net/http"

	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

func (a *API) restoreConvertedStatement(r *http.Request, tx *sql.Tx, householdID, contractID string, original *ledger.Entry) error {
	ctx := r.Context()
	var statementID, instrumentID, occurredOn string
	var converted int64
	err := tx.QueryRowContext(ctx, `
		SELECT MIN(s.id),MIN(s.instrument_id),MIN(i.occurred_on),MIN(i.amount_minor)
		FROM credit_statement_items i JOIN credit_statements s ON s.id=i.statement_id
		WHERE s.household_id=? AND i.source_type='installment_conversion' AND i.source_id=?
		HAVING COUNT(*)=1`, householdID, contractID).Scan(&statementID, &instrumentID, &occurredOn, &converted)
	if errors.Is(err, sql.ErrNoRows) {
		return errInstallmentInUse
	}
	if err != nil {
		return err
	}
	if converted >= 0 ||
		original.Delta(ledger.CreditSubject(instrumentID)) != -money.Minor(converted) ||
		original.Delta(ledger.InstallmentSubject(contractID)) != money.Minor(converted) {
		return errInstallmentInUse
	}

	// Serialize with credit activity that rebuilds the statement from its items.
	query := `SELECT name FROM credit_instruments WHERE household_id=? AND id=?`
	if a.DB.IsMySQL() {
		query += " FOR UPDATE"
	}
	var name string
	if err := tx.QueryRowContext(ctx, query, householdID, instrumentID).Scan(&name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_statements SET system_due_minor=system_due_minor+?
		WHERE household_id=? AND id=?`, -converted, householdID, statementID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_statement_items(statement_id,source_type,source_id,amount_minor,occurred_on)
		VALUES(?,'installment_conversion_reversal',?,?,?)`, statementID, contractID, -converted, occurredOn); err != nil {
		return err
	}

	return a.restoreStatementReminder(ctx, tx, householdID, statementID)
}
