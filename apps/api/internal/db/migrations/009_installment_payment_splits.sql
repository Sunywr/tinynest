-- Backfill pre-008 payments from the immutable debits actually posted.
-- Only rows with no recorded split and a journal split matching the payment qualify.
WITH legacy_splits AS (
    SELECT p.id,
           SUM(CASE WHEN l.account_code = c.ledger_code THEN l.debit_minor ELSE 0 END) AS principal,
           SUM(CASE WHEN l.account_code = '5110' THEN l.debit_minor ELSE 0 END) AS fee
    FROM installment_payments p
    JOIN installment_schedule_items s ON s.id = p.schedule_item_id
    JOIN installment_contracts c ON c.id = s.contract_id
    JOIN journal_entries e ON e.id = p.entry_id AND e.household_id = c.household_id
    JOIN journal_lines l ON l.entry_id = e.id
    WHERE p.principal_minor = 0 AND p.fee_minor = 0
    GROUP BY p.id
)
UPDATE installment_payments AS p
SET principal_minor = legacy.principal, fee_minor = legacy.fee
FROM legacy_splits AS legacy
WHERE p.id = legacy.id AND legacy.principal + legacy.fee = p.amount_minor;
