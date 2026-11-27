-- Copy PFM observed 30-day annualized returns (source unit: percent).
-- Run on the shared MySQL instance with tinynest_v2 selected.
INSERT INTO investment_annual_returns (holding_id, observed_on, annual_return_bp)
SELECT sample.holding_id, sample.observed_on, sample.rate_bp
FROM (
  SELECT h.id AS holding_id, DATE_FORMAT(n.date, '%Y-%m-%d') AS observed_on,
         ROUND(n.annual_30 * 100) AS rate_bp
  FROM investment_holdings h
  JOIN investment_accounts a ON a.id=h.account_id AND a.household_id=h.household_id
  JOIN pfm.account_dailynetvalue n
    ON CONVERT(n.code USING utf8mb4) COLLATE utf8mb4_0900_bin=h.symbol
  WHERE a.kind='wealth' AND a.status='active' AND h.status='active'
    AND n.annual_30 IS NOT NULL
    AND n.date BETWEEN DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR) - INTERVAL 89 DAY
                   AND DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR)
) sample
ON DUPLICATE KEY UPDATE annual_return_bp=sample.rate_bp;
