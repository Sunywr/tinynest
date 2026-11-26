-- 014_statement_details: the credit-card detail rows a household uploads from a
-- bank XLS report (V2 increment). One row is one spreadsheet line, kept verbatim
-- together with the classification the server derived from the bank's own
-- description, so an import can be re-read, re-matched and audited without the
-- file being available again.
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
  id TEXT NOT NULL,
  household_id TEXT NOT NULL REFERENCES households(id),
  account_id TEXT NOT NULL REFERENCES accounts(id),
  statement_id TEXT,
  transaction_id TEXT REFERENCES transactions(id),
  row_key TEXT NOT NULL,
  row_no INTEGER NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0,
  card_tail TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL CHECK (kind IN ('charge','refund','payment','fee','review')),
  state TEXT NOT NULL CHECK (state IN ('imported','matched','held')),
  txn_on TEXT NOT NULL,
  post_on TEXT,
  description TEXT NOT NULL,
  amount_minor INTEGER NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  review_reason TEXT NOT NULL DEFAULT '',
  source_digest TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE (household_id, account_id, row_key, seq),
  UNIQUE (household_id, transaction_id),
  FOREIGN KEY (household_id, statement_id)
    REFERENCES card_statements(household_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_statement_details_account
  ON statement_details(household_id, account_id, txn_on);
