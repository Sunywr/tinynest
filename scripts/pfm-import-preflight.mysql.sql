-- Read-only reconciliation of the two confirmed pfm owners.
-- Totals use integer cents. Count materially fractional cents separately.
WITH source_rows AS (
  SELECT u.username, a.income, a.expenditure, c.display_name, c.name AS category_key,
         a.credit_card_id, a.debt_id, a.invest_id, a.stock_account_id,
         a.transfer_id, a.refund_of_id, a.historical_parent_id
  FROM account_account AS a
  JOIN auth_user AS u ON u.id = a.owner_id_id
  LEFT JOIN account_category AS c ON c.id = a.account_type_id
  WHERE u.username IN ('孙意蔚然', '刘洁')
), classified AS (
  SELECT source_rows.*,
         CASE
           WHEN COALESCE(income, 0) < 0 OR COALESCE(expenditure, 0) < 0 THEN 'negative'
           WHEN COALESCE(income, 0) = 0 AND COALESCE(expenditure, 0) = 0 THEN 'zero'
           WHEN refund_of_id IS NOT NULL THEN 'refund'
           WHEN transfer_id IS NOT NULL OR category_key = 'CASH_TRANSFER' THEN 'transfer'
           WHEN category_key = 'early_loan_repayment' THEN 'credit_repayment'
           WHEN historical_parent_id IS NOT NULL THEN 'historical_child'
           WHEN COALESCE(income, 0) > 0 AND COALESCE(expenditure, 0) > 0 THEN 'both_directions'
           WHEN invest_id IS NOT NULL OR stock_account_id IS NOT NULL
                OR category_key IN ('stock', 'financial', 'stock_account') THEN 'investment'
           WHEN debt_id IS NOT NULL THEN 'debt'
           WHEN credit_card_id IS NOT NULL THEN 'credit'
           ELSE 'ordinary'
         END AS review_group
  FROM source_rows
)
SELECT username, review_group, COUNT(*) AS row_count,
       SUM(CAST(ROUND(COALESCE(income, 0) * 100) AS SIGNED)) AS income_minor,
       SUM(CAST(ROUND(COALESCE(expenditure, 0) * 100) AS SIGNED)) AS expenditure_minor,
       SUM(ABS(COALESCE(income, 0) * 100 - ROUND(COALESCE(income, 0) * 100)) > 0.001
           OR ABS(COALESCE(expenditure, 0) * 100 - ROUND(COALESCE(expenditure, 0) * 100)) > 0.001) AS fractional_cent_rows
FROM classified
GROUP BY username, review_group
ORDER BY username, review_group;
