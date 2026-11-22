package db

import (
	"context"
	"database/sql"
	"fmt"

	"tinynest/api/internal/ledger"
	"tinynest/api/internal/money"
)

// SaveEntry persists a validated, balanced entry. Nothing is written if
// validation fails. Callers MUST already be inside the business transaction so
// the business row and its journal stay atomic.
func SaveEntry(ctx context.Context, tx *sql.Tx, e *ledger.Entry, householdID, idempotencyKey, actor string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	var key any
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO journal_entries
		(id, household_id, biz_type, biz_id, occurred_on, currency, status, reversal_of, idempotency_key, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		e.ID, householdID, e.BizType, e.BizID, e.OccurredOn, e.Currency, e.Status,
		nullString(e.ReversalOf), key, actor)
	if err != nil {
		return fmt.Errorf("db: insert journal entry: %w", err)
	}
	for i, line := range e.Lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO journal_lines
			(entry_id, line_no, account_code, debit_minor, credit_minor, memo) VALUES (?,?,?,?,?,?)`,
			e.ID, i+1, line.Account, int64(line.Debit), int64(line.Credit), line.Memo); err != nil {
			return fmt.Errorf("db: insert journal line %d: %w", i+1, err)
		}
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Balance rebuilds a ledger account balance purely from journal lines
// (debit - credit). The household scope is enforced in SQL: account codes are
// only unique per household, so a global SUM would silently mix two ledgers.
//
// For asset accounts a positive value is the remaining balance; for a liability
// account (credit card) a negative value is the outstanding amount owed.
func Balance(ctx context.Context, q Queryer, householdID, accountCode string) (money.Minor, error) {
	var v int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(l.debit_minor - l.credit_minor), 0)
		 FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		 WHERE l.account_code = ? AND e.household_id = ?`,
		accountCode, householdID).Scan(&v)
	if err != nil {
		return 0, err
	}
	return money.Minor(v), nil
}
