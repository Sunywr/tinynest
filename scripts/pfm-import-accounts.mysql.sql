-- MySQL maintenance import. Default execution is a rollback rehearsal.
START TRANSACTION;

CREATE TEMPORARY TABLE pfm_account_owners AS
SELECT u.id AS user_id,u.household_id,m.member_id,p.id AS pfm_owner
FROM users u
JOIN household_user_members m ON m.user_id=u.id
JOIN pfm.auth_user p ON
  (u.username='sunywr' AND p.username='孙意蔚然') OR
  (u.username='liuj' AND p.username='刘洁');

CREATE TEMPORARY TABLE pfm_accounts AS
SELECT CAST(CONCAT('pfm-cash-',c.id) AS CHAR(191)) AS id,o.household_id,o.user_id,o.member_id,
       CAST('cash' AS CHAR(6)) AS kind,CAST(c.name AS CHAR(191)) AS name,IF(c.enabled=1,'active','disabled') AS status,
       CAST(NULL AS SIGNED) AS statement_day,CAST(NULL AS SIGNED) AS due_day,
       CAST(ROUND(c.opening_balance*100) AS SIGNED) AS opening_minor,
       DATE_FORMAT(c.created_at,'%Y-%m-%d') AS opening_date
FROM pfm.cash_account c JOIN pfm_account_owners o ON o.pfm_owner=c.owner_id;

INSERT INTO pfm_accounts
SELECT CONCAT('pfm-credit-',c.id),o.household_id,o.user_id,o.member_id,
       'credit',c.description,IF(c.status=1,'active','disabled'),
       IF(c.statement_day BETWEEN 1 AND 31,c.statement_day,NULL),
       IF(c.repayment_due_day BETWEEN 1 AND 31,c.repayment_due_day,NULL),0,NULL
FROM pfm.account_creditcard c JOIN pfm_account_owners o ON o.pfm_owner=c.owner_id_id;

DELETE p FROM pfm_accounts p JOIN accounts a ON a.id=p.id;

INSERT INTO accounts (id,household_id,kind,name,owner_member_id,ledger_code,status,statement_day,due_day)
SELECT id,household_id,kind,name,member_id,CONCAT(IF(kind='cash','1010:','2010:'),id),
       status,statement_day,due_day
FROM pfm_accounts;

INSERT INTO journal_entries (id,household_id,biz_type,biz_id,occurred_on,created_by)
SELECT CONCAT('opening-',id),household_id,'opening_cash',id,opening_date,user_id
FROM pfm_accounts WHERE opening_minor<>0;

INSERT INTO journal_lines (entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('opening-',id),1,CONCAT('1010:',id),GREATEST(opening_minor,0),GREATEST(-opening_minor,0)
FROM pfm_accounts WHERE opening_minor<>0;
INSERT INTO journal_lines (entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT CONCAT('opening-',id),2,'3010',GREATEST(-opening_minor,0),GREATEST(opening_minor,0)
FROM pfm_accounts WHERE opening_minor<>0;

INSERT INTO account_opening_balances (id,account_id,amount_minor,entry_id)
SELECT CONCAT('opening-',id),id,ABS(opening_minor),CONCAT('opening-',id)
FROM pfm_accounts WHERE opening_minor<>0;

INSERT INTO audit_logs (household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,user_id,'pfm_import','account',id,JSON_OBJECT('opening_minor',opening_minor)
FROM pfm_accounts;

SELECT kind,status,COUNT(*),SUM(opening_minor) FROM pfm_accounts GROUP BY kind,status;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
