CREATE TABLE IF NOT EXISTS project_budgets (
  id VARCHAR(191) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  name VARCHAR(191) NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  UNIQUE (household_id, name),
  FOREIGN KEY (household_id) REFERENCES households(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
CREATE TABLE IF NOT EXISTS transaction_budget_links (
  transaction_id VARCHAR(191) PRIMARY KEY,
  budget_id VARCHAR(191) NOT NULL,
  INDEX idx_transaction_budget (budget_id),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id),
  FOREIGN KEY (budget_id) REFERENCES project_budgets(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
