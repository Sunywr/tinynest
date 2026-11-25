CREATE TABLE IF NOT EXISTS statement_mailbox_configs (
  household_id TEXT NOT NULL REFERENCES households(id),
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL CHECK (provider IN ('qq','163')),
  email TEXT NOT NULL,
  auth_code_cipher TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version > 0),
  updated_at TEXT NOT NULL,
  PRIMARY KEY (household_id, user_id, provider)
);
CREATE TABLE IF NOT EXISTS statement_mailbox_card_links (
  household_id TEXT NOT NULL,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  PRIMARY KEY (household_id, account_id),
  FOREIGN KEY (household_id, user_id, provider)
    REFERENCES statement_mailbox_configs(household_id, user_id, provider) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_statement_mailbox_card_links_owner
  ON statement_mailbox_card_links(household_id, user_id, provider);
INSERT OR IGNORE INTO statement_mailbox_configs
  (household_id,user_id,provider,email,auth_code_cipher,version,updated_at)
  SELECT household_id,user_id,'qq',email,auth_code_cipher,version,updated_at FROM credit_statement_mailboxes;
INSERT OR IGNORE INTO statement_mailbox_card_links(household_id,account_id,user_id,provider)
  SELECT household_id,account_id,user_id,'qq' FROM credit_statement_bindings;
