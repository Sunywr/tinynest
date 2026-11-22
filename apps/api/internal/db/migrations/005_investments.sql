-- 005: read-only investment valuation snapshot (securities / wealth).
--
-- These tables are a valuation projection imported from the legacy pfm app.
-- They are deliberately separate from accounts and journal_entries: investment
-- market values are not double-entry bookkeeping facts. All amounts are integer
-- minor units, quantities/prices are scaled by 1e6, and return rates are basis
-- points (10000 = 100%).

CREATE TABLE IF NOT EXISTS investment_accounts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  kind TEXT NOT NULL CHECK (kind IN ('security','wealth')),
  name TEXT NOT NULL,
  institution TEXT,
  owner_member_id TEXT,
  source_ref TEXT,
  cash_minor INTEGER NOT NULL DEFAULT 0,
  dividends_minor INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (household_id, source_ref)
);
CREATE INDEX IF NOT EXISTS idx_investment_accounts_household
  ON investment_accounts(household_id, kind, status);

CREATE TABLE IF NOT EXISTS investment_holdings (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  account_id TEXT NOT NULL REFERENCES investment_accounts(id),
  symbol TEXT NOT NULL,
  name TEXT NOT NULL,
  asset_class TEXT NOT NULL DEFAULT '',
  asset_label TEXT NOT NULL DEFAULT '',
  quantity_micros INTEGER NOT NULL DEFAULT 0,
  price_micros INTEGER NOT NULL DEFAULT 0,
  cost_minor INTEGER NOT NULL DEFAULT 0,
  market_value_minor INTEGER NOT NULL DEFAULT 0,
  unrealized_pnl_minor INTEGER NOT NULL DEFAULT 0,
  return_bp INTEGER NOT NULL DEFAULT 0,
  dividends_minor INTEGER NOT NULL DEFAULT 0,
  dividend_yield_bp INTEGER NOT NULL DEFAULT 0,
  last_trade_on TEXT,
  last_trade_pnl_minor INTEGER NOT NULL DEFAULT 0,
  last_trade_return_bp INTEGER NOT NULL DEFAULT 0,
  source_ref TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (account_id, symbol)
);
CREATE INDEX IF NOT EXISTS idx_investment_holdings_household
  ON investment_holdings(household_id, account_id, status);
