-- 004: credit instruments, statements, items, payments
CREATE TABLE IF NOT EXISTS credit_instruments (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('credit_card','huabei','meituan','douyin','custom')),
  name TEXT NOT NULL,
  issuer TEXT,
  member_id TEXT,
  credit_limit_minor INTEGER,
  cycle_start_day INTEGER NOT NULL CHECK (cycle_start_day BETWEEN 1 AND 31),
  cycle_end_day INTEGER NOT NULL CHECK (cycle_end_day BETWEEN 1 AND 31),
  due_day INTEGER NOT NULL CHECK (due_day BETWEEN 1 AND 31),
  due_month_offset INTEGER NOT NULL DEFAULT 1 CHECK (due_month_offset BETWEEN 0 AND 12),
  ledger_code TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (household_id, ledger_code)
);

CREATE TABLE IF NOT EXISTS credit_statements (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  instrument_id TEXT NOT NULL REFERENCES credit_instruments(id),
  cycle_start_on TEXT NOT NULL,
  cycle_end_on TEXT NOT NULL,
  due_on TEXT NOT NULL,
  system_due_minor INTEGER NOT NULL DEFAULT 0,
  paid_minor INTEGER NOT NULL DEFAULT 0,
  platform_amount_minor INTEGER,
  status TEXT NOT NULL DEFAULT 'unbilled',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (instrument_id, cycle_start_on)
);

CREATE TABLE IF NOT EXISTS credit_statement_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  statement_id TEXT NOT NULL REFERENCES credit_statements(id),
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  amount_minor INTEGER NOT NULL,
  occurred_on TEXT NOT NULL,
  UNIQUE (statement_id, source_type, source_id)
);

CREATE TABLE IF NOT EXISTS credit_payments (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  statement_id TEXT NOT NULL REFERENCES credit_statements(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  paid_on TEXT NOT NULL,
  from_account_id TEXT NOT NULL REFERENCES accounts(id),
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_credit_stmt_due ON credit_statements(due_on, status);
