-- MySQL 8 baseline, equivalent to SQLite migrations through 009.

CREATE TABLE IF NOT EXISTS households (
  id VARCHAR(191) PRIMARY KEY,
  name VARCHAR(191) NOT NULL,
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  timezone VARCHAR(191) NOT NULL DEFAULT 'Asia/Shanghai',
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  username VARCHAR(191) NOT NULL UNIQUE,
  password_hash VARCHAR(191) NOT NULL,
  role VARCHAR(191) NOT NULL DEFAULT 'admin',
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS sessions (
  id VARCHAR(191) PRIMARY KEY,
  user_id VARCHAR(191) NOT NULL,
  token_hash VARCHAR(191) NOT NULL UNIQUE,
  csrf_token VARCHAR(191) NOT NULL,
  expires_at VARCHAR(191) NOT NULL,
  revoked_at VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (user_id) REFERENCES users(id),
  INDEX idx_sessions_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS household_members (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS categories (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  parent_id VARCHAR(191),
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('income','expense')),
  name VARCHAR(191) NOT NULL,
  level BIGINT NOT NULL DEFAULT 1 CHECK (level IN (1,2)),
  sort BIGINT NOT NULL DEFAULT 0,
  active BIGINT NOT NULL DEFAULT 1,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (parent_id) REFERENCES categories(id),
  INDEX idx_categories_household (household_id, kind, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS projects (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  start_on VARCHAR(191),
  end_on VARCHAR(191),
  note TEXT,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  household_id VARCHAR(191) NOT NULL,
  actor VARCHAR(191) NOT NULL,
  action VARCHAR(191) NOT NULL,
  entity VARCHAR(191) NOT NULL,
  entity_id VARCHAR(191) NOT NULL,
  payload_json TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  INDEX idx_audit_household (household_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS ledger_accounts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  code VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  category BIGINT NOT NULL CHECK (category BETWEEN 1 AND 5),
  ref_type VARCHAR(191),
  ref_id VARCHAR(191),
  active BIGINT NOT NULL DEFAULT 1,
  UNIQUE (household_id, code),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS journal_entries (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  biz_type VARCHAR(191) NOT NULL,
  biz_id VARCHAR(191) NOT NULL,
  occurred_on VARCHAR(191) NOT NULL,
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  memo VARCHAR(191),
  status VARCHAR(191) NOT NULL DEFAULT 'posted' CHECK (status IN ('posted','reversed')),
  reversal_of VARCHAR(191),
  idempotency_key VARCHAR(191) UNIQUE,
  created_by VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (reversal_of) REFERENCES journal_entries(id),
  INDEX idx_entries_household_date (household_id, occurred_on, biz_type),
  INDEX idx_entries_biz (biz_type, biz_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS journal_lines (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  entry_id VARCHAR(191) NOT NULL,
  line_no BIGINT NOT NULL,
  account_code VARCHAR(191) NOT NULL,
  debit_minor BIGINT NOT NULL DEFAULT 0 CHECK (debit_minor >= 0),
  credit_minor BIGINT NOT NULL DEFAULT 0 CHECK (credit_minor >= 0),
  memo VARCHAR(191),
  CHECK ((debit_minor > 0 AND credit_minor = 0) OR (credit_minor > 0 AND debit_minor = 0)),
  UNIQUE (entry_id, line_no),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_lines_entry (entry_id),
  INDEX idx_lines_account (account_code, entry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS accounts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('cash','securities','wealth')),
  subtype VARCHAR(191),
  name VARCHAR(191) NOT NULL,
  owner_member_id VARCHAR(191),
  ledger_code VARCHAR(191) NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (household_id, ledger_code),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS account_opening_balances (
  id VARCHAR(191) PRIMARY KEY,
  account_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  entry_id VARCHAR(191) NOT NULL,
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS transactions (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  type VARCHAR(191) NOT NULL CHECK (type IN ('expense','income','transfer')),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  occurred_on VARCHAR(191) NOT NULL,
  account_id VARCHAR(191),
  to_account_id VARCHAR(191),
  credit_instrument_id VARCHAR(191),
  member_id VARCHAR(191),
  merchant VARCHAR(191),
  note TEXT,
  status VARCHAR(191) NOT NULL DEFAULT 'posted' CHECK (status IN ('draft','posted','reversed')),
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  created_by VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (to_account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_tx_household_date (household_id, occurred_on, status),
  INDEX idx_tx_account (account_id, occurred_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS transaction_splits (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  transaction_id VARCHAR(191) NOT NULL,
  line_no BIGINT NOT NULL,
  category_id VARCHAR(191) NOT NULL,
  project_id VARCHAR(191),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  note TEXT,
  UNIQUE (transaction_id, line_no),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id),
  FOREIGN KEY (category_id) REFERENCES categories(id),
  FOREIGN KEY (project_id) REFERENCES projects(id),
  INDEX idx_splits_category (category_id),
  INDEX idx_splits_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS refunds (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  transaction_id VARCHAR(191) NOT NULL,
  split_id BIGINT,
  category_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  refund_on VARCHAR(191) NOT NULL,
  destination_type VARCHAR(191) NOT NULL CHECK (destination_type IN ('cash','credit')),
  destination_id VARCHAR(191) NOT NULL,
  tax_unknown BIGINT NOT NULL DEFAULT 0,
  note TEXT,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id),
  FOREIGN KEY (split_id) REFERENCES transaction_splits(id),
  FOREIGN KEY (category_id) REFERENCES categories(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_refunds_tx (transaction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_instruments (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('credit_card','huabei','meituan','douyin','custom')),
  name VARCHAR(191) NOT NULL,
  issuer VARCHAR(191),
  member_id VARCHAR(191),
  credit_limit_minor BIGINT,
  cycle_start_day BIGINT NOT NULL CHECK (cycle_start_day BETWEEN 1 AND 31),
  cycle_end_day BIGINT NOT NULL CHECK (cycle_end_day BETWEEN 1 AND 31),
  due_day BIGINT NOT NULL CHECK (due_day BETWEEN 1 AND 31),
  due_month_offset BIGINT NOT NULL DEFAULT 1 CHECK (due_month_offset BETWEEN 0 AND 12),
  ledger_code VARCHAR(191) NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (household_id, ledger_code),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_statements (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  instrument_id VARCHAR(191) NOT NULL,
  cycle_start_on VARCHAR(191) NOT NULL,
  cycle_end_on VARCHAR(191) NOT NULL,
  due_on VARCHAR(191) NOT NULL,
  system_due_minor BIGINT NOT NULL DEFAULT 0,
  paid_minor BIGINT NOT NULL DEFAULT 0,
  platform_amount_minor BIGINT,
  status VARCHAR(191) NOT NULL DEFAULT 'unbilled',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (instrument_id, cycle_start_on),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (instrument_id) REFERENCES credit_instruments(id),
  INDEX idx_credit_stmt_due (due_on, status),
  INDEX idx_stmt_instrument_due (household_id, due_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_statement_items (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  statement_id VARCHAR(191) NOT NULL,
  source_type VARCHAR(191) NOT NULL,
  source_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL,
  occurred_on VARCHAR(191) NOT NULL,
  UNIQUE (statement_id, source_type, source_id),
  FOREIGN KEY (statement_id) REFERENCES credit_statements(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_payments (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  statement_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  paid_on VARCHAR(191) NOT NULL,
  from_account_id VARCHAR(191) NOT NULL,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (statement_id) REFERENCES credit_statements(id),
  FOREIGN KEY (from_account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS installment_contracts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  source_transaction_id VARCHAR(191),
  name VARCHAR(191) NOT NULL,
  principal_minor BIGINT NOT NULL CHECK (principal_minor > 0),
  total_fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (total_fee_minor >= 0),
  periods BIGINT NOT NULL CHECK (periods > 0),
  first_due_on VARCHAR(191) NOT NULL,
  ledger_code VARCHAR(191) NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (source_transaction_id) REFERENCES transactions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS installment_schedule_items (
  id VARCHAR(191) PRIMARY KEY,
  contract_id VARCHAR(191) NOT NULL,
  installment_no BIGINT NOT NULL,
  due_on VARCHAR(191) NOT NULL,
  principal_minor BIGINT NOT NULL CHECK (principal_minor >= 0),
  fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (fee_minor >= 0),
  total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
  paid_minor BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(191) NOT NULL DEFAULT 'pending',
  UNIQUE (contract_id, installment_no),
  FOREIGN KEY (contract_id) REFERENCES installment_contracts(id),
  INDEX idx_inst_schedule_due (due_on, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS installment_payments (
  id VARCHAR(191) PRIMARY KEY,
  schedule_item_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  paid_on VARCHAR(191) NOT NULL,
  channel VARCHAR(191) NOT NULL CHECK (channel IN ('cash','credit')),
  channel_id VARCHAR(191) NOT NULL,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE
, principal_minor BIGINT NOT NULL DEFAULT 0, fee_minor BIGINT NOT NULL DEFAULT 0, reversed BIGINT NOT NULL DEFAULT 0,
  FOREIGN KEY (schedule_item_id) REFERENCES installment_schedule_items(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS debt_contracts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('mortgage','loan','insurance','other')),
  name VARCHAR(191) NOT NULL,
  principal_minor BIGINT NOT NULL DEFAULT 0,
  remaining_principal_minor BIGINT NOT NULL DEFAULT 0,
  start_on VARCHAR(191),
  end_on VARCHAR(191),
  due_day BIGINT,
  ledger_code VARCHAR(191),
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  note TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ'))
, default_channel VARCHAR(191) NOT NULL DEFAULT 'cash', default_channel_id VARCHAR(191), cycle VARCHAR(191) NOT NULL DEFAULT 'monthly', member_id VARCHAR(191),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS debt_schedule_items (
  id VARCHAR(191) PRIMARY KEY,
  contract_id VARCHAR(191) NOT NULL,
  installment_no BIGINT NOT NULL,
  due_on VARCHAR(191) NOT NULL,
  principal_minor BIGINT NOT NULL DEFAULT 0,
  interest_minor BIGINT NOT NULL DEFAULT 0,
  fee_minor BIGINT NOT NULL DEFAULT 0,
  total_minor BIGINT NOT NULL DEFAULT 0,
  paid_minor BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(191) NOT NULL DEFAULT 'pending',
  UNIQUE (contract_id, installment_no),
  FOREIGN KEY (contract_id) REFERENCES debt_contracts(id),
  INDEX idx_debt_schedule_due (due_on, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS debt_payments (
  id VARCHAR(191) PRIMARY KEY,
  schedule_item_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  paid_on VARCHAR(191) NOT NULL,
  channel VARCHAR(191) NOT NULL CHECK (channel IN ('cash','credit')),
  channel_id VARCHAR(191) NOT NULL,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE
, principal_minor BIGINT NOT NULL DEFAULT 0, interest_minor BIGINT NOT NULL DEFAULT 0, fee_minor BIGINT NOT NULL DEFAULT 0, reversed BIGINT NOT NULL DEFAULT 0,
  FOREIGN KEY (schedule_item_id) REFERENCES debt_schedule_items(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS schedule_due_items (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  source_type VARCHAR(191) NOT NULL,
  source_id VARCHAR(191) NOT NULL,
  biz_key VARCHAR(191) NOT NULL UNIQUE,
  due_on VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL,
  status VARCHAR(191) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','skipped'))
, source_label VARCHAR(191),
  FOREIGN KEY (household_id) REFERENCES households(id),
  INDEX idx_due_items_date (household_id, due_on, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_assets (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  code VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('stock','etf','wealth','custom')),
  market VARCHAR(191),
  account_id VARCHAR(191) NOT NULL,
  ledger_code VARCHAR(191) NOT NULL,
  quantity_scale BIGINT NOT NULL DEFAULT 1000000,
  status VARCHAR(191) NOT NULL DEFAULT 'active',
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  UNIQUE (household_id, code),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (account_id) REFERENCES accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_fee_rules (
  id VARCHAR(191) PRIMARY KEY,
  account_id VARCHAR(191) NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  commission_bps BIGINT NOT NULL DEFAULT 0,
  commission_min_minor BIGINT NOT NULL DEFAULT 0,
  stamp_duty_bps BIGINT NOT NULL DEFAULT 0,
  transfer_fee_bps BIGINT NOT NULL DEFAULT 0,
  other_fee_rule_json TEXT,
  effective_from VARCHAR(191) NOT NULL,
  UNIQUE (account_id, version),
  FOREIGN KEY (account_id) REFERENCES accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_trades (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  asset_id VARCHAR(191) NOT NULL,
  side VARCHAR(191) NOT NULL CHECK (side IN ('buy','sell')),
  trade_on VARCHAR(191) NOT NULL,
  quantity_scaled BIGINT NOT NULL CHECK (quantity_scaled > 0),
  price_scaled BIGINT NOT NULL CHECK (price_scaled > 0),
  gross_minor BIGINT NOT NULL,
  computed_fee_minor BIGINT NOT NULL,
  fee_minor BIGINT NOT NULL,
  fee_rule_id VARCHAR(191),
  fee_rule_snapshot_json TEXT,
  net_minor BIGINT NOT NULL,
  from_account_id VARCHAR(191) NOT NULL,
  position_cost_before_minor BIGINT NOT NULL,
  position_cost_after_minor BIGINT NOT NULL,
  realized_pnl_minor BIGINT NOT NULL DEFAULT 0,
  note TEXT,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (asset_id) REFERENCES investment_assets(id),
  FOREIGN KEY (from_account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_trades_asset (asset_id, trade_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_dividends (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  asset_id VARCHAR(191) NOT NULL,
  ex_on VARCHAR(191),
  received_on VARCHAR(191) NOT NULL,
  gross_minor BIGINT NOT NULL,
  tax_minor BIGINT,                 -- NULL = unknown, NOT zero
  net_minor BIGINT NOT NULL,
  to_account_id VARCHAR(191) NOT NULL,
  to_ledger_code VARCHAR(191) NOT NULL,
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (asset_id) REFERENCES investment_assets(id),
  FOREIGN KEY (to_account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_dividends_year (received_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_valuations (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  asset_id VARCHAR(191) NOT NULL,
  valued_on VARCHAR(191) NOT NULL,
  price_scaled BIGINT NOT NULL,
  market_value_minor BIGINT NOT NULL,
  note TEXT,
  UNIQUE (asset_id, valued_on),
  FOREIGN KEY (asset_id) REFERENCES investment_assets(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS investment_positions (
  asset_id VARCHAR(191) PRIMARY KEY,
  quantity_scaled BIGINT NOT NULL DEFAULT 0,
  total_cost_minor BIGINT NOT NULL DEFAULT 0,
  avg_cost_minor BIGINT NOT NULL DEFAULT 0,
  realized_pnl_minor BIGINT NOT NULL DEFAULT 0,
  net_dividend_minor BIGINT NOT NULL DEFAULT 0,
  last_valued_on VARCHAR(191),
  FOREIGN KEY (asset_id) REFERENCES investment_assets(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_instrument_changes (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  instrument_id VARCHAR(191) NOT NULL,
  changed_fields TEXT NOT NULL,
  reason TEXT,
  actor VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (instrument_id) REFERENCES credit_instruments(id),
  INDEX idx_ci_changes (household_id, instrument_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS credit_statement_adjustments (
  sequence_id BIGINT NOT NULL AUTO_INCREMENT UNIQUE,
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  statement_id VARCHAR(191) NOT NULL,
  field VARCHAR(191) NOT NULL,
  old_value VARCHAR(191) NOT NULL,
  new_value VARCHAR(191) NOT NULL,
  reason TEXT NOT NULL,
  actor VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (statement_id) REFERENCES credit_statements(id),
  INDEX idx_stmt_adj (household_id, statement_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS installment_adjustments (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  contract_id VARCHAR(191) NOT NULL,
  action VARCHAR(191) NOT NULL,
  payload_json TEXT NOT NULL,
  reason TEXT NOT NULL,
  actor VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (contract_id) REFERENCES installment_contracts(id),
  INDEX idx_inst_adj (household_id, contract_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS schedule_runs (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  run_on VARCHAR(191) NOT NULL,
  `trigger` VARCHAR(191) NOT NULL,
  horizon_days BIGINT NOT NULL DEFAULT 30,
  `generated` BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(191) NOT NULL,
  detail TEXT,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  INDEX idx_schedule_runs (household_id, run_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
