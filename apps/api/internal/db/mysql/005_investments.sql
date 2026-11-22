-- 005: read-only investment valuation snapshot (securities / wealth).
--
-- These tables are a valuation projection imported from the legacy pfm app.
-- They are deliberately separate from accounts and journal_entries: investment
-- market values are not double-entry bookkeeping facts. All amounts are integer
-- minor units, quantities/prices are scaled by 1e6, and return rates are basis
-- points (10000 = 100%).
--
-- Re-runnable requirement: CREATE TABLE IF NOT EXISTS with inline indexes only.

CREATE TABLE IF NOT EXISTS investment_accounts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('security','wealth')),
  name VARCHAR(191) NOT NULL,
  institution VARCHAR(191),
  owner_member_id VARCHAR(191),
  source_ref VARCHAR(191),
  cash_minor BIGINT NOT NULL DEFAULT 0,
  dividends_minor BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  updated_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (household_id, source_ref),
  FOREIGN KEY (household_id) REFERENCES households(id),
  INDEX idx_investment_accounts_household (household_id, kind, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_holdings (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  account_id VARCHAR(191) NOT NULL,
  symbol VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  asset_class VARCHAR(191) NOT NULL DEFAULT '',
  asset_label VARCHAR(191) NOT NULL DEFAULT '',
  quantity_micros BIGINT NOT NULL DEFAULT 0,
  price_micros BIGINT NOT NULL DEFAULT 0,
  cost_minor BIGINT NOT NULL DEFAULT 0,
  market_value_minor BIGINT NOT NULL DEFAULT 0,
  unrealized_pnl_minor BIGINT NOT NULL DEFAULT 0,
  return_bp BIGINT NOT NULL DEFAULT 0,
  dividends_minor BIGINT NOT NULL DEFAULT 0,
  dividend_yield_bp BIGINT NOT NULL DEFAULT 0,
  last_trade_on VARCHAR(191),
  last_trade_pnl_minor BIGINT NOT NULL DEFAULT 0,
  last_trade_return_bp BIGINT NOT NULL DEFAULT 0,
  source_ref VARCHAR(191),
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  updated_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (account_id, symbol),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (account_id) REFERENCES investment_accounts(id),
  INDEX idx_investment_holdings_household (household_id, account_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
