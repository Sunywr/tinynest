-- 002: chart of accounts and the immutable double-entry journal
CREATE TABLE IF NOT EXISTS ledger_accounts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  category INTEGER NOT NULL CHECK (category BETWEEN 1 AND 5),
  ref_type TEXT,
  ref_id TEXT,
  active INTEGER NOT NULL DEFAULT 1,
  UNIQUE (household_id, code)
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  biz_type TEXT NOT NULL,
  biz_id TEXT NOT NULL,
  occurred_on TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  memo TEXT,
  status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted','reversed')),
  reversal_of TEXT REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_by TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_entries_household_date ON journal_entries(household_id, occurred_on, biz_type);
CREATE INDEX IF NOT EXISTS idx_entries_biz ON journal_entries(biz_type, biz_id);

CREATE TABLE IF NOT EXISTS journal_lines (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  line_no INTEGER NOT NULL,
  account_code TEXT NOT NULL,
  debit_minor INTEGER NOT NULL DEFAULT 0 CHECK (debit_minor >= 0),
  credit_minor INTEGER NOT NULL DEFAULT 0 CHECK (credit_minor >= 0),
  memo TEXT,
  CHECK ((debit_minor > 0 AND credit_minor = 0) OR (credit_minor > 0 AND debit_minor = 0)),
  UNIQUE (entry_id, line_no)
);
CREATE INDEX IF NOT EXISTS idx_lines_entry ON journal_lines(entry_id);
CREATE INDEX IF NOT EXISTS idx_lines_account ON journal_lines(account_code, entry_id);
