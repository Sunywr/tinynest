-- Refresh already matched pfm investment snapshots. No journal entries are written.
-- Rehearse by default. Prepend SET @pfm_apply=1 to commit.
START TRANSACTION;
CREATE TEMPORARY TABLE pfm_snapshot_members AS
SELECT u.household_id, m.member_id, p.id AS owner_id
FROM users u JOIN household_user_members m ON m.user_id=u.id
JOIN pfm.auth_user p ON (u.username='sunywr' AND p.username='孙意蔚然')
                     OR (u.username='liuj' AND p.username='刘洁');

UPDATE investment_holdings h
JOIN pfm.account_invest i ON h.id=CONVERT(CONCAT('pfm-holding-',i.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
JOIN pfm_snapshot_members m ON m.owner_id=i.owner_id_id AND m.household_id=h.household_id
JOIN investment_accounts a ON a.id=h.account_id AND a.household_id=h.household_id AND a.owner_member_id=m.member_id
SET h.quantity_micros=ROUND(i.share*1000000),
    h.price_micros=ROUND(COALESCE(i.price,0)*1000000),
    h.cost_minor=ROUND(COALESCE(i.cost,0)*100),
    h.market_value_minor=ROUND(COALESCE(i.value,0)*100),
    h.unrealized_pnl_minor=ROUND(COALESCE(i.value,0)*100)-ROUND(COALESCE(i.cost,0)*100),
    h.return_bp=COALESCE(ROUND((i.value-i.cost)/NULLIF(i.cost,0)*10000),0),
    h.updated_at=DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')
WHERE i.status=0 AND i.share>0 AND h.status='active'
  AND (h.quantity_micros<>ROUND(i.share*1000000) OR h.price_micros<>ROUND(COALESCE(i.price,0)*1000000)
       OR h.cost_minor<>ROUND(COALESCE(i.cost,0)*100) OR h.market_value_minor<>ROUND(COALESCE(i.value,0)*100)
       OR h.return_bp<>COALESCE(ROUND((i.value-i.cost)/NULLIF(i.cost,0)*10000),0));
SELECT ROW_COUNT() AS updated_holdings;

UPDATE investment_accounts a
JOIN pfm.account_stockaccount s ON a.id=CONVERT(CONCAT('pfm-security-',s.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
JOIN pfm_snapshot_members m ON m.owner_id=s.owner_id AND m.household_id=a.household_id AND m.member_id=a.owner_member_id
SET a.cash_minor=ROUND(COALESCE(s.available_cash,0)*100),
    a.updated_at=DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')
WHERE a.kind='security' AND a.status='active' AND a.cash_minor<>ROUND(COALESCE(s.available_cash,0)*100);
SELECT ROW_COUNT() AS updated_cash_accounts;

SELECT COUNT(*) AS matched_holdings, SUM(h.market_value_minor) AS market_minor
FROM investment_holdings h JOIN pfm.account_invest i
 ON h.id=CONVERT(CONCAT('pfm-holding-',i.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
JOIN pfm_snapshot_members m ON m.owner_id=i.owner_id_id AND m.household_id=h.household_id
WHERE h.status='active' AND i.status=0 AND i.share>0;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
