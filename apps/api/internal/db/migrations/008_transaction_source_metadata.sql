CREATE TABLE IF NOT EXISTS transaction_source_metadata (
  transaction_id TEXT NOT NULL REFERENCES transactions(id),
  source_id INTEGER NOT NULL,
  item_key TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('description','remark','label')),
  value TEXT NOT NULL,
  PRIMARY KEY(transaction_id,source_id,item_key)
);
