-- 012_card_statements: bank-reported credit-card statements (V2 increment).
-- Rows carry the amounts the BANK reported for one closing period plus the
-- sources that produced them. Nothing here writes journal entries or
-- transactions: the ledger stays the only accounting fact.
--
-- The table names avoid the retired V1 credit_statements /
-- credit_statement_items names on purpose, so the operator purge list keeps its
-- original V1 meaning.

CREATE TABLE IF NOT EXISTS card_statements (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  account_id TEXT NOT NULL REFERENCES accounts(id),
  period_start_on TEXT,
  period_end_on TEXT NOT NULL,
  due_on TEXT,
  currency TEXT NOT NULL DEFAULT 'CNY',
  amount_due_minor INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending_review'
    CHECK (status IN ('pending_review','reconciled')),
  review_reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (household_id, account_id, period_end_on)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_card_statements_household_id
  ON card_statements(household_id, id);
CREATE INDEX IF NOT EXISTS idx_card_statements_account
  ON card_statements(household_id, account_id, period_end_on);

CREATE TABLE IF NOT EXISTS card_statement_sources (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL,
  statement_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('email','xls')),
  provider TEXT,
  user_id TEXT,
  uid INTEGER,
  received_on TEXT,
  digest TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (household_id, digest),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_card_statement_sources_statement
  ON card_statement_sources(household_id, statement_id);
