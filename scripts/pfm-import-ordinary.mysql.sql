-- Import ordinary pfm history into a disabled historical account per member.
-- Run on the production host with the approved MySQL maintenance account.
-- By default this script rehearses all writes and rolls back. Set @pfm_apply=1
-- in the same session ahead of this script after reviewing the result.
START TRANSACTION;

CREATE TEMPORARY TABLE pfm_stage AS
SELECT a.id AS source_id, u.id AS user_id, u.household_id, m.member_id,
       CONCAT('pfm-history-',u.id) AS account_id,
       IF(a.income > 0,'income','expense') AS kind,
       DATE_FORMAT(a.account_date,'%Y-%m-%d') AS occurred_on,
       CAST(ROUND(GREATEST(COALESCE(a.income,0),COALESCE(a.expenditure,0))*100) AS SIGNED) AS amount_minor,
       CASE c.name
         WHEN 'catering' THEN '饮食' WHEN 'daily' THEN '生活'
         WHEN 'entertainment' THEN '娱乐' WHEN 'travel' THEN '旅行'
         WHEN 'study' THEN '教育' WHEN 'relationship' THEN '人情往来'
         ELSE c.display_name
       END AS category_name,
       CONCAT_WS(' · ',NULLIF(a.description,''),NULLIF(a.remark,'')) AS note
FROM pfm.account_account AS a
JOIN pfm.auth_user AS p ON p.id=a.owner_id_id
JOIN users AS u ON u.username=CASE p.username WHEN '孙意蔚然' THEN 'sunywr' WHEN '刘洁' THEN 'liuj' END
JOIN household_user_members AS m ON m.user_id=u.id
JOIN pfm.account_category AS c ON c.id=a.account_type_id
WHERE a.credit_card_id IS NULL AND a.debt_id IS NULL AND a.invest_id IS NULL
  AND a.stock_account_id IS NULL AND a.transfer_id IS NULL AND a.refund_of_id IS NULL
  AND a.historical_parent_id IS NULL AND a.presale_id IS NULL AND a.asset_id IS NULL
  AND a.cash_account_id IS NULL
  AND c.name NOT IN ('financial','stock','stock_account','early_loan_repayment','CASH_TRANSFER')
  AND ((a.income>0 AND COALESCE(a.expenditure,0)=0) OR (a.expenditure>0 AND COALESCE(a.income,0)=0))
  AND ABS(GREATEST(COALESCE(a.income,0),COALESCE(a.expenditure,0))*100-ROUND(GREATEST(COALESCE(a.income,0),COALESCE(a.expenditure,0))*100)) < 0.001;

CREATE TEMPORARY TABLE pfm_ready AS
SELECT s.*, MIN(c.id) AS category_id
FROM pfm_stage s
JOIN categories c ON c.household_id=s.household_id AND c.kind=s.kind
  AND CONVERT(c.name USING utf8mb4) COLLATE utf8mb4_0900_bin =
      CONVERT(s.category_name USING utf8mb4) COLLATE utf8mb4_0900_bin
  AND c.active=1
WHERE s.amount_minor>0 AND s.occurred_on IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM transactions t WHERE t.id=CONCAT('pfm-tx-',s.source_id))
GROUP BY s.source_id,s.user_id,s.household_id,s.member_id,s.account_id,s.kind,s.occurred_on,
         s.amount_minor,s.category_name,s.note
HAVING COUNT(*)=1;

INSERT INTO accounts (id,household_id,kind,name,owner_member_id,ledger_code,status)
SELECT DISTINCT r.account_id,r.household_id,'cash','pfm 历史收支',r.member_id,
       CONCAT('1010:',r.account_id),'disabled'
FROM pfm_ready r
WHERE NOT EXISTS (SELECT 1 FROM accounts a WHERE a.id=r.account_id);

INSERT INTO journal_entries (id,household_id,biz_type,biz_id,occurred_on,created_by)
SELECT CONCAT('pfm-entry-',source_id),household_id,kind,CONCAT('pfm-tx-',source_id),occurred_on,user_id
FROM pfm_ready;

INSERT INTO journal_lines (entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('pfm-entry-',source_id),1,
       IF(kind='income',CONCAT('1010:',account_id),CONCAT('5010:',category_id)),amount_minor,0
FROM pfm_ready;
INSERT INTO journal_lines (entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('pfm-entry-',source_id),2,
       IF(kind='income',CONCAT('4010:',category_id),CONCAT('1010:',account_id)),0,amount_minor
FROM pfm_ready;

INSERT INTO transactions (id,household_id,type,amount_minor,occurred_on,category_id,account_id,member_id,note,entry_id,created_by)
SELECT CONCAT('pfm-tx-',source_id),household_id,kind,amount_minor,occurred_on,category_id,
       account_id,member_id,note,CONCAT('pfm-entry-',source_id),user_id
FROM pfm_ready;

INSERT INTO audit_logs (household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,user_id,'pfm_import','transaction',CONCAT('pfm-tx-',source_id),
       JSON_OBJECT('source','account_account','source_id',source_id)
FROM pfm_ready;

SELECT kind,COUNT(*),SUM(amount_minor) FROM pfm_ready GROUP BY kind;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
