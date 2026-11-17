SELECT t.household_id,t.id,t.entry_id,r.id
FROM transactions t
JOIN journal_entries r ON r.reversal_of=t.entry_id AND r.household_id=t.household_id
WHERE t.status='reversed' AND r.biz_type='reversal'
  AND EXISTS (SELECT 1 FROM audit_logs a
      WHERE a.household_id=t.household_id AND a.entity='transaction'
        AND a.action='reverse_transaction' AND a.entity_id=t.id AND a.payload_json=r.id)
  AND NOT EXISTS (SELECT 1 FROM audit_logs a
      WHERE a.household_id=t.household_id AND a.entity='transaction'
        AND a.action='create_transaction' AND a.payload_json=t.id)
