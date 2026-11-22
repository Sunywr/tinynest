-- 001_core: V2 baseline (SQLite). Identity, household, mail, audit and the
-- immutable double-entry journal. V2 is a baseline reset: this file is only
-- ever applied to an EMPTY database (see docs/plans/v2-cutover-runbook.md).

CREATE TABLE IF NOT EXISTS households (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  status TEXT NOT NULL DEFAULT 'active',
  email TEXT,
  avatar TEXT,
  birthday TEXT,
  payday INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  token_hash TEXT NOT NULL UNIQUE,
  csrf_token TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS household_members (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS household_invites (
  token_hash TEXT PRIMARY KEY,
  household_id TEXT NOT NULL,
  member_id TEXT NOT NULL,
  target_username TEXT,
  expires_at TEXT NOT NULL,
  applicant TEXT,
  decided_at TEXT
);

CREATE TABLE IF NOT EXISTS household_user_members (
  user_id TEXT PRIMARY KEY,
  member_id TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS pending_registrations (
  username TEXT PRIMARY KEY,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS site_administrators (user_id TEXT PRIMARY KEY);

CREATE TABLE IF NOT EXISTS email_identities (
  email TEXT PRIMARY KEY,
  owner TEXT NOT NULL UNIQUE,
  verified_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS email_challenges (
  id TEXT PRIMARY KEY,
  owner TEXT NOT NULL,
  email TEXT NOT NULL,
  purpose TEXT NOT NULL,
  digest TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  consumed INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS email_challenge_owner ON email_challenges(owner,purpose,created_at);

CREATE TABLE IF NOT EXISTS mail_configuration (
  id INTEGER PRIMARY KEY,
  version INTEGER NOT NULL,
  config_json TEXT NOT NULL,
  password_cipher TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mail_preferences (
  user_id TEXT PRIMARY KEY,
  announcements INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS mail_daily_usage (
  day TEXT PRIMARY KEY,
  total INTEGER NOT NULL DEFAULT 0,
  announcements INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS mail_recipient_daily_usage (
  day TEXT NOT NULL,
  email TEXT NOT NULL,
  total INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(day,email)
);

CREATE TABLE IF NOT EXISTS mail_queue (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  recipient TEXT NOT NULL,
  kind TEXT NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  due_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS mail_queue_status ON mail_queue(status,due_at);

CREATE TABLE IF NOT EXISTS audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  household_id TEXT NOT NULL,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  entity TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  payload_json TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_audit_household ON audit_logs(household_id, created_at);

CREATE TABLE IF NOT EXISTS ledger_accounts (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  category INTEGER NOT NULL CHECK (category BETWEEN 1 AND 5),
  ref_type TEXT,
  ref_id TEXT,
  active INTEGER NOT NULL DEFAULT 1,
  UNIQUE (household_id, code)
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  biz_type TEXT NOT NULL,
  biz_id TEXT NOT NULL,
  occurred_on TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  memo TEXT,
  status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted','reversed')),
  reversal_of TEXT REFERENCES journal_entries(id),
  idempotency_key TEXT UNIQUE,
  created_by TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_entries_household_date ON journal_entries(household_id, occurred_on, biz_type);
CREATE INDEX IF NOT EXISTS idx_entries_biz ON journal_entries(biz_type, biz_id);

CREATE TABLE IF NOT EXISTS journal_lines (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entry_id TEXT NOT NULL REFERENCES journal_entries(id),
  line_no INTEGER NOT NULL,
  account_code TEXT NOT NULL,
  debit_minor INTEGER NOT NULL DEFAULT 0 CHECK (debit_minor >= 0),
  credit_minor INTEGER NOT NULL DEFAULT 0 CHECK (credit_minor >= 0),
  memo TEXT,
  CHECK ((debit_minor > 0 AND credit_minor = 0) OR (credit_minor > 0 AND debit_minor = 0)),
  UNIQUE (entry_id, line_no)
);
CREATE INDEX IF NOT EXISTS idx_lines_entry ON journal_lines(entry_id);
CREATE INDEX IF NOT EXISTS idx_lines_account ON journal_lines(account_code, entry_id);

-- References accounts(id), which is created by 002_bookkeeping. SQLite resolves
-- foreign keys lazily, so the forward reference is safe here.
CREATE TABLE IF NOT EXISTS account_opening_balances (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  entry_id TEXT NOT NULL REFERENCES journal_entries(id)
);

CREATE TABLE IF NOT EXISTS schema_meta (
  meta_key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
