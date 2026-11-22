-- 004: per-credit-account statement close day and repayment due day.
--
-- MySQL DDL commits implicitly, so this file must be re-runnable after a crash
-- that happened between an ALTER and the schema_migrations insert. MySQL 8 has
-- no ADD COLUMN IF NOT EXISTS, so each column is checked independently in
-- information_schema and added through a prepared statement. Independent checks
-- also recover a crash that added only one of the two columns.
SET @add_statement_day := (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE accounts ADD COLUMN statement_day INT NULL',
    'DO 0')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'accounts' AND column_name = 'statement_day'
);
PREPARE add_statement_day_stmt FROM @add_statement_day;
EXECUTE add_statement_day_stmt;
DEALLOCATE PREPARE add_statement_day_stmt;

SET @add_due_day := (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE accounts ADD COLUMN due_day INT NULL',
    'DO 0')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'accounts' AND column_name = 'due_day'
);
PREPARE add_due_day_stmt FROM @add_due_day;
EXECUTE add_due_day_stmt;
DEALLOCATE PREPARE add_due_day_stmt;
