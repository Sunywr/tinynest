-- 001_core: V2 baseline (MySQL 8). Identity, household, mail, audit and the
-- immutable double-entry journal. V2 is a baseline reset: this file is only
-- ever applied to an EMPTY database (see docs/plans/v2-cutover-runbook.md).
--
-- Re-runnable requirement: MySQL DDL commits implicitly, so the migrator may
-- have to run this whole file again after a crash. Every statement below is
-- therefore idempotent (CREATE TABLE IF NOT EXISTS with inline indexes; no
-- ALTER, no CREATE INDEX).

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
  email VARCHAR(254) NULL,
  avatar TEXT NULL,
  birthday CHAR(10) NULL,
  payday INT NULL,
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

CREATE TABLE IF NOT EXISTS household_invites (
  token_hash VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  member_id VARCHAR(191) NOT NULL,
  target_username VARCHAR(100),
  expires_at VARCHAR(32) NOT NULL,
  applicant VARCHAR(100),
  decided_at VARCHAR(32)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS household_user_members (
  user_id VARCHAR(191) PRIMARY KEY,
  member_id VARCHAR(191) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS pending_registrations (
  username VARCHAR(100) PRIMARY KEY,
  password_hash VARCHAR(191) NOT NULL,
  created_at VARCHAR(32) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS site_administrators (user_id VARCHAR(64) PRIMARY KEY) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS email_identities (
  email VARCHAR(254) PRIMARY KEY,
  owner VARCHAR(320) NOT NULL UNIQUE,
  verified_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS email_challenges (
  id VARCHAR(64) PRIMARY KEY,
  owner VARCHAR(320) NOT NULL,
  email VARCHAR(254) NOT NULL,
  purpose VARCHAR(32) NOT NULL,
  digest VARCHAR(64) NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  created_at VARCHAR(40) NOT NULL,
  expires_at VARCHAR(40) NOT NULL,
  consumed INTEGER NOT NULL DEFAULT 0,
  INDEX email_challenge_owner (owner,purpose,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS mail_configuration (
  id INTEGER PRIMARY KEY,
  version INTEGER NOT NULL,
  config_json TEXT NOT NULL,
  password_cipher TEXT NOT NULL,
  updated_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS mail_preferences (
  user_id VARCHAR(64) PRIMARY KEY,
  announcements INTEGER NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS mail_daily_usage (
  day VARCHAR(10) PRIMARY KEY,
  total INTEGER NOT NULL DEFAULT 0,
  announcements INTEGER NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS mail_recipient_daily_usage (
  day VARCHAR(10) NOT NULL,
  email VARCHAR(254) NOT NULL,
  total INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(day,email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS mail_queue (
  id VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  recipient VARCHAR(254) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  payload MEDIUMTEXT NOT NULL,
  status VARCHAR(32) NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  due_at VARCHAR(40) NOT NULL,
  expires_at VARCHAR(40) NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  INDEX mail_queue_status (status,due_at)
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

-- `accounts` is created by 002_bookkeeping; MySQL requires a foreign key target
-- to exist, so the account reference is enforced by the application instead of
-- a database constraint in this dialect. The journal_entries reference is kept.
CREATE TABLE IF NOT EXISTS account_opening_balances (
  id VARCHAR(191) PRIMARY KEY,
  account_id VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  entry_id VARCHAR(191) NOT NULL,
  FOREIGN KEY (entry_id) REFERENCES journal_entries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS schema_meta (
  meta_key VARCHAR(64) PRIMARY KEY,
  value TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
