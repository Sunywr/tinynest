-- Production maintenance. Default execution rehearses and rolls back.
START TRANSACTION;
CREATE TEMPORARY TABLE pfm_members AS
SELECT u.id AS actor,u.household_id,m.member_id,p.id AS owner_id
FROM users u JOIN household_user_members m ON m.user_id=u.id
JOIN pfm.auth_user p ON (u.username='sunywr' AND p.username='孙意蔚然')
                     OR (u.username='liuj' AND p.username='刘洁');

CREATE TEMPORARY TABLE pfm_disabled AS
SELECT a.id,a.household_id,MIN(m.actor) AS actor
FROM accounts a JOIN pfm_members m ON m.household_id=a.household_id
WHERE a.status='disabled' GROUP BY a.id,a.household_id;

INSERT INTO audit_logs(household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,actor,'delete_account','account',id,JSON_OBJECT('source','pfm_cleanup')
FROM pfm_disabled;
UPDATE accounts a JOIN pfm_disabled d ON d.id=a.id AND d.household_id=a.household_id
SET a.status='deleted' WHERE a.status='disabled';

CREATE TEMPORARY TABLE pfm_snapshot_accounts AS
SELECT CAST(CONCAT('pfm-security-',p.id) AS CHAR(191)) AS id,m.household_id,m.member_id,m.owner_id,
       'security' AS kind,CAST(p.name AS CHAR(191)) AS name,p.institution_name AS institution,
       CAST(ROUND(COALESCE(p.available_cash,0)*100) AS SIGNED) AS cash_minor
FROM pfm.account_stockaccount p JOIN pfm_members m ON m.owner_id=p.owner_id
WHERE (SELECT COUNT(*) FROM pfm.account_stockaccount s WHERE s.owner_id=p.owner_id)=1;

INSERT INTO pfm_snapshot_accounts
SELECT CONCAT('pfm-wealth-',m.owner_id),m.household_id,m.member_id,m.owner_id,
       'wealth','pfm 理财',NULL,0
FROM pfm_members m
WHERE EXISTS (SELECT 1 FROM pfm.account_invest i WHERE i.owner_id_id=m.owner_id
              AND i.status=0 AND i.invest_type_id<>1 AND i.share>0);

INSERT INTO investment_accounts(id,household_id,kind,name,institution,owner_member_id,source_ref,cash_minor)
SELECT id,household_id,kind,name,institution,member_id,id,cash_minor FROM pfm_snapshot_accounts s
WHERE NOT EXISTS (SELECT 1 FROM investment_accounts a WHERE a.id=s.id);

INSERT INTO investment_holdings(id,household_id,account_id,symbol,name,asset_class,asset_label,
   quantity_micros,price_micros,cost_minor,market_value_minor,unrealized_pnl_minor,return_bp,source_ref)
SELECT CONCAT('pfm-holding-',i.id),s.household_id,s.id,
       COALESCE(NULLIF(i.code,''),CONCAT('pfm-',i.id)),COALESCE(NULLIF(i.description,''),i.code),
       IF(s.kind='security','stock','wealth'),IF(s.kind='security','证券','理财'),
       ROUND(i.share*1000000),ROUND(COALESCE(i.price,0)*1000000),
       ROUND(COALESCE(i.cost,0)*100),ROUND(COALESCE(i.value,0)*100),
       ROUND(COALESCE(i.value,0)*100)-ROUND(COALESCE(i.cost,0)*100),
       COALESCE(ROUND((i.value-i.cost)/NULLIF(i.cost,0)*10000),0),CONCAT('account_invest:',i.id)
FROM pfm.account_invest i
JOIN pfm_snapshot_accounts s ON s.owner_id=i.owner_id_id
  AND s.kind=IF(i.invest_type_id=1,'security','wealth')
WHERE i.status=0 AND i.share>0
  AND NOT EXISTS (SELECT 1 FROM investment_holdings h WHERE h.id=CONCAT('pfm-holding-',i.id));

SELECT COUNT(*) AS deleted_accounts FROM pfm_disabled;
SELECT a.kind,COUNT(DISTINCT a.id),COUNT(h.id),SUM(h.market_value_minor)
FROM investment_accounts a LEFT JOIN investment_holdings h ON h.account_id=a.id AND h.household_id=a.household_id
JOIN pfm_snapshot_accounts s ON s.id=a.id AND s.household_id=a.household_id
GROUP BY a.kind;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
