-- 002_bookkeeping: V2 baseline (MySQL 8). The public "流水" model on top of the
-- double-entry kernel: single-level categories, cash/credit accounts, one
-- category per transaction, monthly budgets.
--
-- Re-runnable requirement: every statement below is idempotent (CREATE TABLE IF
-- NOT EXISTS with inline indexes; no ALTER, no CREATE INDEX). The MySQL
-- migrator may re-run the whole file after a crash that happened between the
-- last DDL statement and the schema_migrations write.

CREATE TABLE IF NOT EXISTS categories (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('income','expense')),
  name VARCHAR(191) NOT NULL,
  emoji VARCHAR(16) CHARACTER SET utf8mb4 NOT NULL DEFAULT '',
  sort BIGINT NOT NULL DEFAULT 0,
  active BIGINT NOT NULL DEFAULT 1,
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  INDEX idx_categories_household (household_id, kind, active, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS accounts (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  kind VARCHAR(191) NOT NULL CHECK (kind IN ('cash','credit')),
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

CREATE TABLE IF NOT EXISTS transactions (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  type VARCHAR(191) NOT NULL CHECK (type IN ('expense','income','transfer')),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  currency VARCHAR(191) NOT NULL DEFAULT 'CNY',
  occurred_on VARCHAR(191) NOT NULL,
  category_id VARCHAR(191),
  account_id VARCHAR(191),
  to_account_id VARCHAR(191),
  member_id VARCHAR(191),
  note TEXT,
  status VARCHAR(191) NOT NULL DEFAULT 'posted' CHECK (status IN ('posted','reversed')),
  entry_id VARCHAR(191) NOT NULL,
  idempotency_key VARCHAR(191) UNIQUE,
  created_by VARCHAR(191),
  created_at VARCHAR(191) NOT NULL DEFAULT (DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (category_id) REFERENCES categories(id),
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (to_account_id) REFERENCES accounts(id),
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id),
  INDEX idx_tx_household_date (household_id, occurred_on, status),
  INDEX idx_tx_category (household_id, category_id, occurred_on),
  INDEX idx_tx_account (account_id, occurred_on)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS budgets (
  household_id VARCHAR(191) NOT NULL,
  month VARCHAR(7) NOT NULL,
  amount_minor BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (household_id, month),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
