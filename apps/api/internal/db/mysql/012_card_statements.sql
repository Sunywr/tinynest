-- 012_card_statements: bank-reported credit-card statements (V2 increment).
-- Repeatable by design: both tables use IF NOT EXISTS, so re-running this file
-- on an existing V2 database is a no-op (MySQL commits DDL implicitly).
--
-- The table names avoid the retired V1 credit_statements /
-- credit_statement_items names on purpose, so the operator purge list keeps its
-- original V1 meaning.

CREATE TABLE IF NOT EXISTS card_statements (
  id VARCHAR(191) NOT NULL,
  household_id VARCHAR(191) NOT NULL,
  account_id VARCHAR(191) NOT NULL,
  period_start_on VARCHAR(191),
  period_end_on VARCHAR(191) NOT NULL,
  due_on VARCHAR(191),
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  amount_due_minor BIGINT NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'pending_review' CHECK (status IN ('pending_review','reconciled')),
  review_reason VARCHAR(191) NOT NULL DEFAULT '',
  created_at VARCHAR(191) NOT NULL,
  updated_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_card_statements_period (household_id, account_id, period_end_on),
  UNIQUE KEY uq_card_statements_household_id (household_id, id),
  INDEX idx_card_statements_account (household_id, account_id, period_end_on),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (account_id) REFERENCES accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
CREATE TABLE IF NOT EXISTS card_statement_sources (
  id VARCHAR(191) NOT NULL,
  household_id VARCHAR(191) NOT NULL,
  statement_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('email','xls')),
  provider VARCHAR(191),
  user_id VARCHAR(191),
  uid BIGINT,
  received_on VARCHAR(191),
  digest VARCHAR(191) NOT NULL,
  created_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_card_statement_sources_digest (household_id, digest),
  INDEX idx_card_statement_sources_statement (household_id, statement_id),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
