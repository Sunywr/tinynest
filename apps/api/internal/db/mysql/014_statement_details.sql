-- 014_statement_details: the credit-card detail rows a household uploads from a
-- bank XLS report (V2 increment).
--
-- Repeatable by design: the table uses IF NOT EXISTS with inline indexes and
-- constraints, so re-running this file on an existing V2 database is a no-op
-- (MySQL commits DDL implicitly).
--
-- The accounting fact still lives only in journal_entries / journal_lines: this
-- table records which lines became a transaction (state=imported), which lines the
-- ledger already held (state=matched) and which lines were deliberately NOT booked
-- (state=held, with review_reason). Only kind='charge' may become a 5010 expense;
-- refunds, repayments/transfers, fees and anything unrecognised are held, because a
-- negative amount in a bank report is not proof of a refund.
--
-- Idempotency is per LINE, not per file: row_key+seq is derived from the reported
-- date, amount, card tail and description, so re-downloading the same statement, or
-- uploading it again after one line was corrected, cannot post an expense twice.
-- seq keeps two genuinely identical charges on one day representable instead of
-- silently collapsing them.
CREATE TABLE IF NOT EXISTS statement_details (
  id VARCHAR(191) NOT NULL,
  household_id VARCHAR(191) NOT NULL,
  account_id VARCHAR(191) NOT NULL,
  statement_id VARCHAR(191),
  transaction_id VARCHAR(191),
  row_key VARCHAR(191) NOT NULL,
  row_no BIGINT NOT NULL,
  seq BIGINT NOT NULL DEFAULT 0,
  card_tail VARCHAR(191) NOT NULL DEFAULT '',
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('charge','refund','payment','fee','review')),
  state VARCHAR(191) NOT NULL CHECK (state IN ('imported','matched','held')),
  txn_on VARCHAR(191) NOT NULL,
  post_on VARCHAR(191),
  description TEXT NOT NULL,
  amount_minor BIGINT NOT NULL,
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  review_reason VARCHAR(191) NOT NULL DEFAULT '',
  source_digest VARCHAR(191) NOT NULL DEFAULT '',
  created_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_statement_details_row (household_id, account_id, row_key, seq),
  UNIQUE KEY uq_statement_details_transaction (household_id, transaction_id),
  INDEX idx_statement_details_account (household_id, account_id, txn_on),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
