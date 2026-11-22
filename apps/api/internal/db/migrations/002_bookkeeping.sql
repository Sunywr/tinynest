-- 002_bookkeeping: V2 baseline (SQLite). The public "流水" model on top of the
-- double-entry kernel: single-level categories, cash/credit accounts, one
-- category per transaction, monthly budgets.

CREATE TABLE IF NOT EXISTS categories (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('income','expense')),
  name TEXT NOT NULL,
  emoji TEXT NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_categories_household ON categories(household_id, kind, active, sort);

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('cash','credit')),
  subtype TEXT,
  name TEXT NOT NULL,
  owner_member_id TEXT,
  ledger_code TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (household_id, ledger_code)
);

CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  type TEXT NOT NULL CHECK (type IN ('expense','income','transfer')),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  occurred_on TEXT NOT NULL,
  category_id TEXT REFERENCES categories(id),
  account_id TEXT REFERENCES accounts(id),
  to_account_id TEXT REFERENCES accounts(id),
  member_id TEXT,
  note TEXT,
  status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted','reversed')),
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_by TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_tx_household_date ON transactions(household_id, occurred_on, status);
CREATE INDEX IF NOT EXISTS idx_tx_category ON transactions(household_id, category_id, occurred_on);
CREATE INDEX IF NOT EXISTS idx_tx_account ON transactions(account_id, occurred_on);

CREATE TABLE IF NOT EXISTS budgets (
  household_id TEXT NOT NULL REFERENCES households(id),
  month TEXT NOT NULL,
  amount_minor INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (household_id, month)
);
