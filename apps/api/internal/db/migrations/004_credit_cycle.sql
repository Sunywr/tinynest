-- 004: per-credit-account statement close day and repayment due day.
-- SQLite runs a migration and its ledger row in one transaction, so plain
-- ADD COLUMN statements are atomic and need no separate existence guard.
ALTER TABLE accounts ADD COLUMN statement_day INTEGER;
ALTER TABLE accounts ADD COLUMN due_day INTEGER;
