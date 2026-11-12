-- 005: installment contracts, debt contracts, schedule items, due items
CREATE TABLE IF NOT EXISTS installment_contracts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  source_transaction_id TEXT REFERENCES transactions(id),
  name TEXT NOT NULL,
  principal_minor INTEGER NOT NULL CHECK (principal_minor > 0),
  total_fee_minor INTEGER NOT NULL DEFAULT 0 CHECK (total_fee_minor >= 0),
  periods INTEGER NOT NULL CHECK (periods > 0),
  first_due_on TEXT NOT NULL,
  ledger_code TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS installment_schedule_items (
  id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL REFERENCES installment_contracts(id),
  installment_no INTEGER NOT NULL,
  due_on TEXT NOT NULL,
  principal_minor INTEGER NOT NULL CHECK (principal_minor >= 0),
  fee_minor INTEGER NOT NULL DEFAULT 0 CHECK (fee_minor >= 0),
  total_minor INTEGER NOT NULL CHECK (total_minor >= 0),
  paid_minor INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  UNIQUE (contract_id, installment_no)
);

CREATE TABLE IF NOT EXISTS installment_payments (
  id TEXT PRIMARY KEY,
  schedule_item_id TEXT NOT NULL REFERENCES installment_schedule_items(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  paid_on TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('cash','credit')),
  channel_id TEXT NOT NULL,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE
);

CREATE TABLE IF NOT EXISTS debt_contracts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('mortgage','loan','insurance','other')),
  name TEXT NOT NULL,
  principal_minor INTEGER NOT NULL DEFAULT 0,
  remaining_principal_minor INTEGER NOT NULL DEFAULT 0,
  start_on TEXT,
  end_on TEXT,
  due_day INTEGER,
  ledger_code TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS debt_schedule_items (
  id TEXT PRIMARY KEY,
  contract_id TEXT NOT NULL REFERENCES debt_contracts(id),
  installment_no INTEGER NOT NULL,
  due_on TEXT NOT NULL,
  principal_minor INTEGER NOT NULL DEFAULT 0,
  interest_minor INTEGER NOT NULL DEFAULT 0,
  fee_minor INTEGER NOT NULL DEFAULT 0,
  total_minor INTEGER NOT NULL DEFAULT 0,
  paid_minor INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  UNIQUE (contract_id, installment_no)
);

CREATE TABLE IF NOT EXISTS debt_payments (
  id TEXT PRIMARY KEY,
  schedule_item_id TEXT NOT NULL REFERENCES debt_schedule_items(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  paid_on TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('cash','credit')),
  channel_id TEXT NOT NULL,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE
);

-- Idempotent projection of future obligations: a scheduled task only ever INSERTs pending rows.
CREATE TABLE IF NOT EXISTS schedule_due_items (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  biz_key TEXT NOT NULL UNIQUE,
  due_on TEXT NOT NULL,
  amount_minor INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','skipped'))
);
CREATE INDEX IF NOT EXISTS idx_due_items_date ON schedule_due_items(household_id, due_on, status);
