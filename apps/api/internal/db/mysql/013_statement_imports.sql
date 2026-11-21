CREATE TABLE IF NOT EXISTS statement_import_rows (
  household_id VARCHAR(64) NOT NULL,
  id VARCHAR(64) NOT NULL,
  source VARCHAR(24) NOT NULL,
  profile VARCHAR(120) NOT NULL,
  uploaded_by VARCHAR(64) NOT NULL,
  binding_key VARCHAR(64) NOT NULL,
  reference_hash VARCHAR(64) NOT NULL,
  payload TEXT NOT NULL,
  transaction_id VARCHAR(64),
  resolution VARCHAR(24) NOT NULL DEFAULT '',
  PRIMARY KEY (household_id, id),
  INDEX idx_statement_import_tx (household_id, transaction_id)
);

CREATE TABLE IF NOT EXISTS statement_import_bindings (
  household_id VARCHAR(64) NOT NULL,
  binding_key VARCHAR(64) NOT NULL,
  account_id VARCHAR(64) NOT NULL,
  card_id VARCHAR(64) NOT NULL,
  PRIMARY KEY (household_id, binding_key)
);
