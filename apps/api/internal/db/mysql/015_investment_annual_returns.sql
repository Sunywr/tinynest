CREATE TABLE IF NOT EXISTS investment_annual_returns (
  holding_id VARCHAR(191) NOT NULL,
  observed_on VARCHAR(10) NOT NULL,
  annual_return_bp BIGINT NOT NULL,
  PRIMARY KEY (holding_id, observed_on),
  FOREIGN KEY (holding_id) REFERENCES investment_holdings(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
