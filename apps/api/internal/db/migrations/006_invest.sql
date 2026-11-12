-- 006: investment assets, fee rules, trades, dividends, valuations, positions
CREATE TABLE IF NOT EXISTS investment_assets (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('stock','etf','wealth','custom')),
  market TEXT,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  ledger_code TEXT NOT NULL,
  quantity_scale INTEGER NOT NULL DEFAULT 1000000,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (household_id, code)
);

CREATE TABLE IF NOT EXISTS investment_fee_rules (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  version INTEGER NOT NULL DEFAULT 1,
  commission_bps INTEGER NOT NULL DEFAULT 0,
  commission_min_minor INTEGER NOT NULL DEFAULT 0,
  stamp_duty_bps INTEGER NOT NULL DEFAULT 0,
  transfer_fee_bps INTEGER NOT NULL DEFAULT 0,
  other_fee_rule_json TEXT,
  effective_from TEXT NOT NULL,
  UNIQUE (account_id, version)
);

CREATE TABLE IF NOT EXISTS investment_trades (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  asset_id TEXT NOT NULL REFERENCES investment_assets(id),
  side TEXT NOT NULL CHECK (side IN ('buy','sell')),
  trade_on TEXT NOT NULL,
  quantity_scaled INTEGER NOT NULL CHECK (quantity_scaled > 0),
  price_scaled INTEGER NOT NULL CHECK (price_scaled > 0),
  gross_minor INTEGER NOT NULL,
  computed_fee_minor INTEGER NOT NULL,
  fee_minor INTEGER NOT NULL,
  fee_rule_id TEXT,
  fee_rule_snapshot_json TEXT,
  net_minor INTEGER NOT NULL,
  from_account_id TEXT NOT NULL REFERENCES accounts(id),
  position_cost_before_minor INTEGER NOT NULL,
  position_cost_after_minor INTEGER NOT NULL,
  realized_pnl_minor INTEGER NOT NULL DEFAULT 0,
  note TEXT,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_trades_asset ON investment_trades(asset_id, trade_on);

CREATE TABLE IF NOT EXISTS investment_dividends (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  asset_id TEXT NOT NULL REFERENCES investment_assets(id),
  ex_on TEXT,
  received_on TEXT NOT NULL,
  gross_minor INTEGER NOT NULL,
  tax_minor INTEGER,                 -- NULL = unknown, NOT zero
  net_minor INTEGER NOT NULL,
  to_account_id TEXT NOT NULL REFERENCES accounts(id),
  to_ledger_code TEXT NOT NULL,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_dividends_year ON investment_dividends(received_on);

CREATE TABLE IF NOT EXISTS investment_valuations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  asset_id TEXT NOT NULL REFERENCES investment_assets(id),
  valued_on TEXT NOT NULL,
  price_scaled INTEGER NOT NULL,
  market_value_minor INTEGER NOT NULL,
  note TEXT,
  UNIQUE (asset_id, valued_on)
);

-- Rebuildable projection; the journal remains the single source of truth.
CREATE TABLE IF NOT EXISTS investment_positions (
  asset_id TEXT PRIMARY KEY REFERENCES investment_assets(id),
  quantity_scaled INTEGER NOT NULL DEFAULT 0,
  total_cost_minor INTEGER NOT NULL DEFAULT 0,
  avg_cost_minor INTEGER NOT NULL DEFAULT 0,
  realized_pnl_minor INTEGER NOT NULL DEFAULT 0,
  net_dividend_minor INTEGER NOT NULL DEFAULT 0,
  last_valued_on TEXT
);
