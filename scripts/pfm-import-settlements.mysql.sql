-- Rehearse by default. Set @pfm_apply=1 in the same session to commit.
START TRANSACTION;
CREATE TEMPORARY TABLE pfm_settlement AS
SELECT e.id AS source_id,NULLIF(i.id,0) AS paired_id,CAST('transfer' AS CHAR(32)) AS biz_type,
       CAST(CONCAT('pfm-cash-',e.cash_account_id) AS CHAR(191)) AS from_id,
       CAST(CONCAT('pfm-cash-',i.cash_account_id) AS CHAR(191)) AS to_id,
       e.owner_id_id AS owner_id,DATE_FORMAT(e.account_date,'%Y-%m-%d') AS occurred_on,
       CAST(ROUND(e.expenditure*100) AS SIGNED) AS amount_minor,
       CAST(CONCAT_WS(' · ',e.description,e.remark) AS CHAR(2000)) AS note
FROM pfm.account_account e JOIN pfm.account_account i ON i.transfer_id=e.transfer_id
JOIN (SELECT transfer_id FROM pfm.account_account WHERE transfer_id IS NOT NULL
      GROUP BY transfer_id HAVING COUNT(*)=2) paired ON paired.transfer_id=e.transfer_id
WHERE e.expenditure>0 AND i.income=e.expenditure
  AND COALESCE(e.income,0)=0 AND COALESCE(i.expenditure,0)=0
  AND e.owner_id_id=i.owner_id_id AND DATE(e.account_date)=DATE(i.account_date);

INSERT INTO pfm_settlement
SELECT a.id,NULL,'credit_repayment',CONCAT('pfm-cash-',a.cash_account_id),
       CONCAT('pfm-credit-',COALESCE(a.credit_card_id,d.credit_card_id)),a.owner_id_id,
       DATE_FORMAT(a.account_date,'%Y-%m-%d'),ROUND(a.expenditure*100),
       CONCAT_WS(' · ',a.description,a.remark)
FROM pfm.account_account a JOIN pfm.account_category c ON c.id=a.account_type_id
LEFT JOIN pfm.account_debt d ON d.id=a.debt_id AND d.owner_id_id=a.owner_id_id
WHERE c.name='early_loan_repayment' AND a.expenditure>0 AND COALESCE(a.income,0)=0
  AND a.cash_account_id IS NOT NULL
  AND COALESCE(a.credit_card_id,d.credit_card_id) IS NOT NULL
  AND (a.credit_card_id IS NULL OR d.credit_card_id IS NULL OR a.credit_card_id=d.credit_card_id);

CREATE TEMPORARY TABLE pfm_ready AS
SELECT s.*,u.id AS actor,u.household_id,m.member_id,f.ledger_code AS from_code,t.ledger_code AS to_code
FROM pfm_settlement s
JOIN pfm.auth_user p ON p.id=s.owner_id
JOIN users u ON u.username=CASE p.username WHEN '孙意蔚然' THEN 'sunywr' WHEN '刘洁' THEN 'liuj' END
JOIN household_user_members m ON m.user_id=u.id
JOIN accounts f ON f.id=s.from_id AND f.household_id=u.household_id AND f.owner_member_id=m.member_id
JOIN accounts t ON t.id=s.to_id AND t.household_id=u.household_id AND t.owner_member_id=m.member_id
WHERE f.id<>t.id AND f.kind='cash' AND t.kind=IF(s.biz_type='transfer','cash','credit')
  AND s.amount_minor>0
  AND NOT EXISTS (SELECT 1 FROM transactions x
                  WHERE x.id IN(CONCAT('pfm-tx-',s.source_id),CONCAT('pfm-tx-',s.paired_id)));
INSERT INTO journal_entries(id,household_id,biz_type,biz_id,occurred_on,created_by)
SELECT CONCAT('pfm-entry-',source_id),household_id,biz_type,CONCAT('pfm-tx-',source_id),occurred_on,actor FROM pfm_ready;
INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('pfm-entry-',source_id),1,to_code,amount_minor,0 FROM pfm_ready;
INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('pfm-entry-',source_id),2,from_code,0,amount_minor FROM pfm_ready;
INSERT INTO transactions(id,household_id,type,amount_minor,occurred_on,account_id,to_account_id,member_id,note,entry_id,created_by)
SELECT CONCAT('pfm-tx-',source_id),household_id,'transfer',amount_minor,occurred_on,
       from_id,to_id,member_id,note,CONCAT('pfm-entry-',source_id),actor FROM pfm_ready;
INSERT INTO audit_logs(household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,actor,'pfm_import','transaction',CONCAT('pfm-tx-',source_id),
       JSON_OBJECT('source','account_account','source_id',source_id,'paired_id',paired_id)
FROM pfm_ready;
SELECT biz_type,COUNT(*),SUM(amount_minor) FROM pfm_ready GROUP BY biz_type;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
