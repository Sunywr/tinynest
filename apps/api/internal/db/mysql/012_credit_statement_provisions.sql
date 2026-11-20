CREATE TABLE IF NOT EXISTS credit_statement_provisions (
  statement_id VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(64) NOT NULL,
  target_minor BIGINT NOT NULL,
  provision_minor BIGINT NOT NULL DEFAULT 0,
  entry_id VARCHAR(64) NOT NULL DEFAULT ''
);
