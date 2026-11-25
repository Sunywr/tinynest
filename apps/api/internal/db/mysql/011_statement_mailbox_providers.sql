CREATE TABLE IF NOT EXISTS statement_mailbox_configs (
  household_id VARCHAR(191) NOT NULL,
  user_id VARCHAR(191) NOT NULL,
  provider VARCHAR(8) NOT NULL CHECK (provider IN ('qq','163')),
  email VARCHAR(254) NOT NULL,
  auth_code_cipher TEXT NOT NULL,
  version BIGINT NOT NULL CHECK (version > 0),
  updated_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (household_id, user_id, provider),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
CREATE TABLE IF NOT EXISTS statement_mailbox_card_links (
  household_id VARCHAR(191) NOT NULL,
  account_id VARCHAR(191) NOT NULL,
  user_id VARCHAR(191) NOT NULL,
  provider VARCHAR(8) NOT NULL,
  PRIMARY KEY (household_id, account_id),
  INDEX idx_statement_mailbox_card_links_owner (household_id, user_id, provider),
  FOREIGN KEY (account_id) REFERENCES accounts(id),
  FOREIGN KEY (household_id, user_id, provider)
    REFERENCES statement_mailbox_configs(household_id, user_id, provider) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
INSERT IGNORE INTO statement_mailbox_configs
  (household_id,user_id,provider,email,auth_code_cipher,version,updated_at)
  SELECT household_id,user_id,'qq',email,auth_code_cipher,version,updated_at FROM credit_statement_mailboxes;
INSERT IGNORE INTO statement_mailbox_card_links(household_id,account_id,user_id,provider)
  SELECT household_id,account_id,user_id,'qq' FROM credit_statement_bindings;
