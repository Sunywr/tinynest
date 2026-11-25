CREATE TABLE IF NOT EXISTS credit_statement_bindings (
  household_id VARCHAR(191) NOT NULL,
  account_id VARCHAR(191) NOT NULL,
  user_id VARCHAR(191) NOT NULL,
  PRIMARY KEY (household_id, account_id),
  INDEX idx_statement_bindings_mailbox (household_id, user_id),
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (household_id, user_id)
    REFERENCES credit_statement_mailboxes(household_id, user_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
