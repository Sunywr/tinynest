CREATE TABLE IF NOT EXISTS credit_statement_bindings (
  household_id TEXT NOT NULL,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  user_id TEXT NOT NULL,
  PRIMARY KEY (household_id, account_id),
  FOREIGN KEY (household_id, user_id)
    REFERENCES credit_statement_mailboxes(household_id, user_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_statement_bindings_mailbox ON credit_statement_bindings(household_id, user_id);
