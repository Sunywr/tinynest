CREATE TABLE IF NOT EXISTS credit_statement_mailboxes (
  household_id VARCHAR(191) NOT NULL,
  user_id VARCHAR(191) NOT NULL,
  email VARCHAR(254) NOT NULL,
  auth_code_cipher TEXT NOT NULL,
  version BIGINT NOT NULL CHECK (version > 0),
  updated_at VARCHAR(191) NOT NULL,
  PRIMARY KEY (household_id, user_id),
  FOREIGN KEY (household_id) REFERENCES households(id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
