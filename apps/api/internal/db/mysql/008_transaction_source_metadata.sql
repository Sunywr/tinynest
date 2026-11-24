CREATE TABLE IF NOT EXISTS transaction_source_metadata (
  transaction_id VARCHAR(191) NOT NULL,
  source_id BIGINT NOT NULL,
  item_key VARCHAR(64) NOT NULL,
  kind VARCHAR(16) NOT NULL CHECK(kind IN ('description','remark','label')),
  value TEXT NOT NULL,
  PRIMARY KEY(transaction_id,source_id,item_key),
  FOREIGN KEY(transaction_id) REFERENCES transactions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
