-- 013_statement_placeholders: the 待补明细 placeholder that stands in for
-- credit-card details a household never uploaded. One row pins one bank
-- statement (card_statements) to the transaction that carries its 应还 amount,
-- together with the period and repayment deadline the BANK reported.
--
-- The accounting fact still lives only in journal_entries / journal_lines; this
-- table is the link plus the statement's own dates, which the repayment plan
-- needs because the placeholder is dated at the deadline while the bill covers
-- the cycle that closed before it. The primary key is what makes creating a
-- placeholder idempotent: at most one per statement, ever.

CREATE TABLE IF NOT EXISTS statement_placeholders (
  household_id TEXT NOT NULL REFERENCES households(id),
  statement_id TEXT NOT NULL,
  transaction_id TEXT NOT NULL REFERENCES transactions(id),
  period_start_on TEXT,
  period_end_on TEXT NOT NULL,
  due_on TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (household_id, statement_id),
  UNIQUE (household_id, transaction_id),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_statement_placeholders_due
  ON statement_placeholders(household_id, due_on);
