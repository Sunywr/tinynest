-- Correct labelled or uniquely matched PFM household transfers imported as income/expense.
-- Run with imports and ledger writes paused. Rehearses unless @pfm_apply=1.
START TRANSACTION;

CREATE TEMPORARY TABLE pfm_family_candidates AS
SELECT STRAIGHT_JOIN t.*, f.ledger_code AS cash_code,
       CONVERT(s.description USING utf8mb4) COLLATE utf8mb4_0900_bin AS source_description,
       COALESCE(TRIM(s.description),'')='' AND COALESCE(TRIM(s.remark),'')='' AS no_memo,
       CASE owner.username
         WHEN 'sunywr' THEN IF(t.type='income','老婆转账','转账给老婆')
         WHEN 'liuj' THEN IF(t.type='income','老公转账','转给老公')
       END COLLATE utf8mb4_0900_bin AS spouse_description,
       CONCAT('pfm-family-clearing-',t.household_id) AS clearing_id,
       CONCAT('pfm-family-transfer-',s.id) AS replacement_id,
       CONCAT('pfm-family-entry-',s.id) AS replacement_entry,
       CONCAT('pfm-family-reversal-',s.id) AS reversal_entry
FROM pfm.account_account s
JOIN pfm.account_category c ON c.id=s.account_type_id AND c.name='family'
JOIN pfm.auth_user source_owner ON source_owner.id=s.owner_id_id
JOIN users owner ON owner.username=CASE source_owner.username
  WHEN '孙意蔚然' THEN 'sunywr' WHEN '刘洁' THEN 'liuj' END
JOIN household_user_members member ON member.user_id=owner.id
JOIN users spouse ON spouse.household_id=owner.household_id
  AND spouse.username=CASE owner.username WHEN 'sunywr' THEN 'liuj' WHEN 'liuj' THEN 'sunywr' END
JOIN household_user_members spouse_member ON spouse_member.user_id=spouse.id
  AND spouse_member.member_id<>member.member_id
JOIN transactions t ON t.id=CONVERT(CONCAT('pfm-tx-',s.id) USING utf8mb4) COLLATE utf8mb4_0900_bin
  AND t.household_id=owner.household_id AND t.member_id=member.member_id AND t.status='posted'
JOIN accounts f ON f.id=t.account_id AND f.household_id=t.household_id AND f.kind='cash'
JOIN journal_entries e ON e.id=t.entry_id AND e.household_id=t.household_id
  AND e.status='posted' AND e.reversal_of IS NULL
WHERE s.transfer_id IS NULL AND s.refund_of_id IS NULL
  AND t.occurred_on=DATE_FORMAT(s.account_date,'%Y-%m-%d')
  AND (
    (t.type='income' AND s.income>0 AND COALESCE(s.expenditure,0)=0
      AND t.amount_minor=ROUND(s.income*100))
    OR
    (t.type='expense' AND s.expenditure>0 AND COALESCE(s.income,0)=0
      AND t.amount_minor=ROUND(s.expenditure*100))
  )
;

-- Materialize the narrow source set before matching to keep MySQL from
-- optimizing the source joins separately for each side of the pair.
CREATE TEMPORARY TABLE pfm_family_incoming AS
SELECT id,household_id,member_id,occurred_on,amount_minor
FROM pfm_family_candidates
WHERE type='income' AND (no_memo OR source_description=spouse_description);

CREATE TEMPORARY TABLE pfm_family_pairs AS
SELECT outgoing.id AS expense_id, incoming.id AS income_id,
       COUNT(*) OVER (PARTITION BY outgoing.id) AS expense_matches,
       COUNT(*) OVER (PARTITION BY incoming.id) AS income_matches
FROM pfm_family_candidates outgoing
JOIN pfm_family_incoming incoming ON incoming.household_id=outgoing.household_id
  AND incoming.member_id<>outgoing.member_id
  AND incoming.occurred_on=outgoing.occurred_on
  AND incoming.amount_minor=outgoing.amount_minor
WHERE outgoing.type='expense'
  AND (outgoing.no_memo OR outgoing.source_description=outgoing.spouse_description);

CREATE TEMPORARY TABLE pfm_family_correction AS
SELECT c.* FROM pfm_family_candidates c
LEFT JOIN pfm_family_pairs p ON p.expense_matches=1 AND p.income_matches=1
  AND c.id IN (p.expense_id,p.income_id)
WHERE c.source_description=c.spouse_description OR p.expense_id IS NOT NULL;

-- One disabled clearing account per household preserves each known cash leg.
-- Missing opposite source legs are never fabricated in real member accounts.
INSERT INTO accounts(id,household_id,kind,name,ledger_code,status)
SELECT DISTINCT r.clearing_id,r.household_id,'cash','家庭内部转账',
       CONCAT('1010:',r.clearing_id),'disabled'
FROM pfm_family_correction r
WHERE NOT EXISTS (SELECT 1 FROM accounts a WHERE a.id=r.clearing_id);

INSERT INTO journal_entries(id,household_id,biz_type,biz_id,occurred_on,currency,reversal_of,created_by)
SELECT reversal_entry,household_id,'reversal',id,occurred_on,currency,entry_id,created_by
FROM pfm_family_correction;

INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT r.reversal_entry,l.line_no,l.account_code,l.credit_minor,l.debit_minor
FROM pfm_family_correction r JOIN journal_lines l ON l.entry_id=r.entry_id;

INSERT INTO journal_entries(id,household_id,biz_type,biz_id,occurred_on,currency,created_by)
SELECT replacement_entry,household_id,'transfer',replacement_id,occurred_on,currency,created_by
FROM pfm_family_correction;

INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT replacement_entry,1,IF(type='income',cash_code,CONCAT('1010:',clearing_id)),amount_minor,0
FROM pfm_family_correction;
INSERT INTO journal_lines(entry_id,line_no,account_code,debit_minor,credit_minor)
SELECT replacement_entry,2,IF(type='expense',cash_code,CONCAT('1010:',clearing_id)),0,amount_minor
FROM pfm_family_correction;

INSERT INTO transactions(id,household_id,type,amount_minor,currency,occurred_on,account_id,to_account_id,member_id,note,entry_id,created_by)
SELECT replacement_id,household_id,'transfer',amount_minor,currency,occurred_on,
       IF(type='expense',account_id,clearing_id),IF(type='income',account_id,clearing_id),
       member_id,note,replacement_entry,created_by
FROM pfm_family_correction;

INSERT INTO transaction_source_metadata(transaction_id,source_id,item_key,kind,value)
SELECT r.replacement_id,m.source_id,m.item_key,m.kind,m.value
FROM pfm_family_correction r JOIN transaction_source_metadata m ON m.transaction_id=r.id;

UPDATE transactions t JOIN pfm_family_correction r ON r.id=t.id AND r.household_id=t.household_id
SET t.status='reversed';
UPDATE journal_entries e JOIN pfm_family_correction r ON r.entry_id=e.id AND r.household_id=e.household_id
SET e.status='reversed';

INSERT INTO audit_logs(household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,created_by,'reverse_transaction','transaction',id,reversal_entry
FROM pfm_family_correction;
INSERT INTO audit_logs(household_id,actor,action,entity,entity_id,payload_json)
SELECT household_id,created_by,'correct_transaction','transaction',replacement_id,id
FROM pfm_family_correction;

SELECT type,COUNT(*) AS corrected,SUM(amount_minor) AS amount_minor
FROM pfm_family_correction GROUP BY type;
SET @finish := IF(COALESCE(@pfm_apply,0)=1,'COMMIT','ROLLBACK');
PREPARE pfm_finish FROM @finish;
EXECUTE pfm_finish;
DEALLOCATE PREPARE pfm_finish;
