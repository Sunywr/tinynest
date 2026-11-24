-- Rehearses by default. Prepend SET @pfm_apply=1 to commit on the shared MySQL instance.
START TRANSACTION;

CREATE TEMPORARY TABLE pfm_metadata_roots AS
SELECT STRAIGHT_JOIN s.id AS source_id,t.id AS transaction_id,t.household_id
FROM pfm.account_account s
JOIN pfm.auth_user p ON p.id=s.owner_id_id
JOIN users u ON u.username=CASE p.username WHEN '孙意蔚然' THEN 'sunywr' WHEN '刘洁' THEN 'liuj' END
JOIN household_user_members m ON m.user_id=u.id
JOIN transactions t ON t.id=CONVERT(CONCAT('pfm-tx-',s.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
    AND t.household_id=u.household_id AND t.member_id=m.member_id;

-- Settlement imports save both source IDs in the audit event.
INSERT INTO pfm_metadata_roots
SELECT STRAIGHT_JOIN s.id,t.id,t.household_id
FROM audit_logs a
JOIN transactions t ON t.id=a.entity_id AND t.household_id=a.household_id AND t.type='transfer'
JOIN pfm.account_account s ON s.id=CAST(JSON_UNQUOTE(JSON_EXTRACT(
    IF(JSON_VALID(a.payload_json),a.payload_json,'{}'),'$.paired_id')) AS UNSIGNED)
JOIN pfm.auth_user p ON p.id=s.owner_id_id
JOIN users u ON u.username=CASE p.username WHEN '孙意蔚然' THEN 'sunywr' WHEN '刘洁' THEN 'liuj' END
    AND u.household_id=t.household_id
JOIN accounts destination ON destination.id=t.to_account_id AND destination.household_id=t.household_id
JOIN household_user_members m ON m.user_id=u.id AND m.member_id=destination.owner_member_id
WHERE a.action='pfm_import' AND a.entity='transaction'
  AND JSON_TYPE(JSON_EXTRACT(IF(JSON_VALID(a.payload_json),a.payload_json,'{}'),'$.paired_id'))='INTEGER';

CREATE TEMPORARY TABLE pfm_metadata_corrections (
    INDEX idx_old_tx(household_id,old_id)
) AS
SELECT household_id,entity_id,CAST(payload_json AS CHAR(191)) AS old_id
FROM audit_logs
WHERE action='correct_transaction' AND entity='transaction' AND payload_json IS NOT NULL;

CREATE TEMPORARY TABLE pfm_metadata_targets AS
WITH RECURSIVE history AS (
    SELECT source_id,transaction_id,household_id FROM pfm_metadata_roots
    UNION DISTINCT
    SELECT h.source_id,t.id,h.household_id
    FROM history h
    JOIN pfm_metadata_corrections a ON a.household_id=h.household_id AND a.old_id=h.transaction_id
    JOIN transactions t ON t.id=a.entity_id AND t.household_id=h.household_id
)
SELECT h.source_id,t.id AS transaction_id
FROM history h JOIN transactions t ON t.id=h.transaction_id AND t.household_id=h.household_id
WHERE t.status='posted';

CREATE TEMPORARY TABLE pfm_metadata_stage AS
SELECT t.transaction_id,t.source_id,CAST('description' AS CHAR(64)) AS item_key,
       CAST('description' AS CHAR(16)) AS kind,CAST(s.description AS CHAR(16383)) AS value
FROM pfm_metadata_targets t JOIN pfm.account_account s ON s.id=t.source_id
WHERE TRIM(COALESCE(s.description,''))<>'';

INSERT INTO pfm_metadata_stage
SELECT t.transaction_id,t.source_id,'remark','remark',s.remark
FROM pfm_metadata_targets t JOIN pfm.account_account s ON s.id=t.source_id
WHERE TRIM(COALESCE(s.remark,''))<>'';

INSERT INTO pfm_metadata_stage
SELECT DISTINCT t.transaction_id,t.source_id,CONCAT('label:',l.id),'label',l.name
FROM pfm_metadata_targets t
JOIN pfm.account_accountlabel x ON x.account_id_id=t.source_id
JOIN pfm.account_label l ON l.id=x.label_id_id
WHERE TRIM(COALESCE(l.name,''))<>'';

INSERT INTO transaction_source_metadata(transaction_id,source_id,item_key,kind,value)
SELECT DISTINCT s.transaction_id,s.source_id,s.item_key,s.kind,s.value
FROM pfm_metadata_stage s
WHERE NOT EXISTS (SELECT 1 FROM transaction_source_metadata m
    WHERE m.transaction_id=s.transaction_id AND m.source_id=s.source_id AND m.item_key=s.item_key);

SELECT ROW_COUNT() AS inserted_metadata;
SELECT COUNT(DISTINCT transaction_id) AS target_bills,COUNT(DISTINCT source_id) AS pfm_rows
FROM pfm_metadata_targets;
SELECT kind,COUNT(*) AS source_items,COUNT(DISTINCT transaction_id) AS bills
FROM pfm_metadata_stage GROUP BY kind;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
