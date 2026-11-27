package api

import (
	"context"

	"tinynest/api/internal/money"
)

// statementExpenseAdjustment replaces covered card expenses with the bank's
// dated consumption rows. The XLS source is recorded only on confirmation.
func (a *API) statementExpenseAdjustment(ctx context.Context, householdID, from, to string) (money.Minor, error) {
	var delta int64
	err := a.DB.QueryRowContext(ctx, `
 WITH coverage AS (
  SELECT s.id, s.household_id, s.account_id, s.period_start_on, s.period_end_on
  FROM card_statements s
  WHERE s.household_id=? AND s.currency='CNY' AND s.period_start_on IS NOT NULL
    AND EXISTS (SELECT 1 FROM card_statement_sources src
      WHERE src.household_id=s.household_id AND src.statement_id=s.id AND src.kind='xls')
    AND EXISTS (SELECT 1 FROM statement_details detail
      WHERE detail.household_id=s.household_id AND detail.account_id=s.account_id AND detail.statement_id=s.id
        AND detail.kind='charge' AND detail.currency='CNY' AND detail.amount_minor>0
        AND detail.txn_on BETWEEN s.period_start_on AND s.period_end_on)
 )
 SELECT
  (SELECT COALESCE(SUM(d.amount_minor),0)
   FROM statement_details d JOIN coverage s
     ON s.household_id=d.household_id AND s.id=d.statement_id AND s.account_id=d.account_id
   WHERE d.kind='charge' AND d.currency='CNY' AND d.amount_minor>0
     AND d.txn_on BETWEEN ? AND ? AND d.txn_on BETWEEN s.period_start_on AND s.period_end_on
     AND NOT EXISTS (SELECT 1 FROM coverage newer
       WHERE newer.account_id=s.account_id AND newer.period_end_on>s.period_end_on
         AND d.txn_on BETWEEN newer.period_start_on AND newer.period_end_on))
  -
  (SELECT COALESCE(SUM(l.debit_minor-l.credit_minor),0)
   FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
   JOIN transactions t ON t.household_id=e.household_id AND t.id=e.biz_id
   WHERE e.household_id=? AND e.occurred_on BETWEEN ? AND ?
     AND l.account_code LIKE '5010:%' AND e.reversal_of IS NULL AND t.status='posted' AND t.type='expense'
     AND (
       EXISTS (SELECT 1 FROM coverage s WHERE s.account_id=t.account_id
         AND e.occurred_on BETWEEN s.period_start_on AND s.period_end_on)
       OR EXISTS (SELECT 1 FROM statement_placeholders p JOIN coverage s
         ON s.household_id=p.household_id AND s.id=p.statement_id
         WHERE p.household_id=t.household_id AND p.transaction_id=t.id)
     ))`, householdID, from, to, householdID, from, to).Scan(&delta)
	return money.Minor(delta), err
}
