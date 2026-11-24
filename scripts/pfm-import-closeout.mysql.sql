-- Read-only closeout. Run against pfm on the shared MySQL instance.
SELECT a.id AS source_id,COALESCE(t.id,paired.id) AS target_id,
       a.credit_card_id,a.debt_id,a.invest_id,a.stock_account_id,
       CAST(ROUND(COALESCE(a.income,0)*100) AS SIGNED) AS income_minor,
       CAST(ROUND(COALESCE(a.expenditure,0)*100) AS SIGNED) AS expenditure_minor,
       CASE
         WHEN t.id IS NOT NULL THEN 'imported'
         WHEN paired.id IS NOT NULL THEN 'merged_transfer'
         WHEN COALESCE(a.income,0)=0 AND COALESCE(a.expenditure,0)=0 THEN 'ignored_zero_amount'
         WHEN a.refund_of_id IS NOT NULL THEN 'retained_refund_requires_original_expense_adjustment'
         WHEN a.income<0 OR a.expenditure<0 THEN 'retained_negative_requires_source_evidence'
         WHEN a.income>0 AND a.expenditure>0 THEN 'retained_mixed_directions'
         WHEN c.name='early_loan_repayment' THEN 'retained_repayment_reverse_direction'
         WHEN c.name IN ('stock','financial','stock_account') OR a.invest_id IS NOT NULL
              OR a.stock_account_id IS NOT NULL THEN 'retained_investment_history'
         WHEN dc.name='credit_card' THEN 'retained_credit_cycle_pending_balance_reconciliation'
         WHEN dc.name='installment' THEN 'retained_installment_to_avoid_duplicate_expense'
         WHEN a.debt_id IS NOT NULL THEN 'retained_debt_cash_flow_unverified'
         WHEN a.credit_card_id IS NOT NULL THEN 'retained_credit_cash_flow_unverified'
         WHEN ABS(COALESCE(a.income,0)*100-ROUND(COALESCE(a.income,0)*100))>0.001
           OR ABS(COALESCE(a.expenditure,0)*100-ROUND(COALESCE(a.expenditure,0)*100))>0.001
           THEN 'retained_fractional_cent'
         WHEN a.asset_id IS NOT NULL OR a.presale_id IS NOT NULL THEN 'retained_unsupported_asset_reference'
         ELSE 'retained_category_or_business_meaning_unverified'
       END AS disposition
FROM account_account a
JOIN auth_user u ON u.id=a.owner_id_id
LEFT JOIN account_category c ON c.id=a.account_type_id
LEFT JOIN account_debt d ON d.id=a.debt_id AND d.owner_id_id=a.owner_id_id
LEFT JOIN account_debtcategory dc ON dc.id=d.debt_type_id
LEFT JOIN tinynest_v2.transactions t
  ON t.id=CONVERT(CONCAT('pfm-tx-',a.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
LEFT JOIN (
  SELECT DISTINCT id,transfer_id,expenditure,owner_id_id,account_date
  FROM account_account WHERE transfer_id IS NOT NULL AND expenditure>0
) e ON a.income>0 AND e.transfer_id=a.transfer_id
  AND e.expenditure=a.income AND e.owner_id_id=a.owner_id_id AND DATE(e.account_date)=DATE(a.account_date)
LEFT JOIN tinynest_v2.transactions paired
  ON paired.id=CONVERT(CONCAT('pfm-tx-',e.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
  AND paired.type='transfer' AND paired.amount_minor=ROUND(a.income*100)
WHERE u.username IN ('孙意蔚然','刘洁')
ORDER BY a.id;
