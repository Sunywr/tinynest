-- 013_statement_placeholders: the 待补明细 placeholder that stands in for
-- credit-card details a household never uploaded. One row pins one bank
-- statement (card_statements) to the transaction that carries its 应还 amount,
-- together with the period and repayment deadline the BANK reported.
--
-- Repeatable by design: the table uses IF NOT EXISTS, so re-running this file on
-- an existing V2 database is a no-op (MySQL commits DDL implicitly).
--
-- The accounting fact still lives only in journal_entries / journal_lines; this
-- table is the link plus the statement's own dates, which the repayment plan
-- needs because the placeholder is dated at the deadline while the bill covers
-- the cycle that closed before it. The primary key is what makes creating a
-- placeholder idempotent: at most one per statement, ever.
CREATE TABLE IF NOT EXISTS statement_placeholders (
  household_id VARCHAR(191) NOT NULL,
  statement_id VARCHAR(191) NOT NULL,
  transaction_id VARCHAR(191) NOT NULL,
  period_start_on VARCHAR(191),
  period_end_on VARCHAR(191) NOT NULL,
  due_on VARCHAR(191) NOT NULL,
  created_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (household_id, statement_id),
  UNIQUE KEY uq_statement_placeholders_transaction (household_id, transaction_id),
  INDEX idx_statement_placeholders_due (household_id, due_on),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
