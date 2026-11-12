-- 003: accounts, openings, transactions, splits, refunds
CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('cash','securities','wealth')),
  subtype TEXT,
  name TEXT NOT NULL,
  owner_member_id TEXT,
  ledger_code TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (household_id, ledger_code)
);

CREATE TABLE IF NOT EXISTS account_opening_balances (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  entry_id TEXT NOT NULL REFERENCES journal_entries(id)
);

CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  type TEXT NOT NULL CHECK (type IN ('expense','income','transfer')),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  occurred_on TEXT NOT NULL,
  account_id TEXT REFERENCES accounts(id),
  to_account_id TEXT REFERENCES accounts(id),
  credit_instrument_id TEXT,
  member_id TEXT,
  merchant TEXT,
  note TEXT,
  status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('draft','posted','reversed')),
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_by TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_tx_household_date ON transactions(household_id, occurred_on, status);
CREATE INDEX IF NOT EXISTS idx_tx_account ON transactions(account_id, occurred_on);

CREATE TABLE IF NOT EXISTS transaction_splits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  transaction_id TEXT NOT NULL REFERENCES transactions(id),
  line_no INTEGER NOT NULL,
  category_id TEXT NOT NULL REFERENCES categories(id),
  project_id TEXT REFERENCES projects(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  note TEXT,
  UNIQUE (transaction_id, line_no)
);
CREATE INDEX IF NOT EXISTS idx_splits_category ON transaction_splits(category_id);
CREATE INDEX IF NOT EXISTS idx_splits_project ON transaction_splits(project_id);

CREATE TABLE IF NOT EXISTS refunds (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  transaction_id TEXT NOT NULL REFERENCES transactions(id),
  split_id INTEGER REFERENCES transaction_splits(id),
  category_id TEXT NOT NULL REFERENCES categories(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  refund_on TEXT NOT NULL,
  destination_type TEXT NOT NULL CHECK (destination_type IN ('cash','credit')),
  destination_id TEXT NOT NULL,
  tax_unknown INTEGER NOT NULL DEFAULT 0,
  note TEXT,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_refunds_tx ON refunds(transaction_id);
