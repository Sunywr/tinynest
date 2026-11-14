-- 008: V0.1.1 financial closure. APPEND ONLY - never rewrite 001..007.
-- All monetary values are INTEGER minor units (cents). Every business row is
-- scoped by household_id either directly or through its parent contract.

-- Explicit, user-confirmed payment splits. V0.1.1 must NOT auto-allocate a
-- partial payment between principal, interest and fee (10 undecided policies).
ALTER TABLE installment_payments ADD COLUMN principal_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE installment_payments ADD COLUMN fee_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE installment_payments ADD COLUMN reversed INTEGER NOT NULL DEFAULT 0;

ALTER TABLE debt_payments ADD COLUMN principal_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE debt_payments ADD COLUMN interest_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE debt_payments ADD COLUMN fee_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE debt_payments ADD COLUMN reversed INTEGER NOT NULL DEFAULT 0;

-- Fixed-obligation contracts: payment channel + recurrence metadata.
ALTER TABLE debt_contracts ADD COLUMN default_channel TEXT NOT NULL DEFAULT 'cash';
ALTER TABLE debt_contracts ADD COLUMN default_channel_id TEXT;
ALTER TABLE debt_contracts ADD COLUMN cycle TEXT NOT NULL DEFAULT 'monthly';
ALTER TABLE debt_contracts ADD COLUMN member_id TEXT;

-- Due-item projection carries a readable label for the UI drill-down.
ALTER TABLE schedule_due_items ADD COLUMN source_label TEXT;

-- Audit trail for configuration changes. A cycle edit must NEVER silently
-- recompute an already-issued statement; the old value is retained here.
CREATE TABLE IF NOT EXISTS credit_instrument_changes (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  instrument_id TEXT NOT NULL REFERENCES credit_instruments(id),
  changed_fields TEXT NOT NULL,
  reason TEXT,
  actor TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_ci_changes ON credit_instrument_changes(household_id, instrument_id);

CREATE TABLE IF NOT EXISTS credit_statement_adjustments (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  statement_id TEXT NOT NULL REFERENCES credit_statements(id),
  field TEXT NOT NULL,
  old_value TEXT NOT NULL,
  new_value TEXT NOT NULL,
  reason TEXT NOT NULL,
  actor TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_stmt_adj ON credit_statement_adjustments(household_id, statement_id);

-- Installment schedule changes are explicit and fully traceable. Paid periods
-- are never deleted; only future pending periods may be cancelled/re-planned.
CREATE TABLE IF NOT EXISTS installment_adjustments (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  contract_id TEXT NOT NULL REFERENCES installment_contracts(id),
  action TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  reason TEXT NOT NULL,
  actor TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_inst_adj ON installment_adjustments(household_id, contract_id);

-- Daily scheduler run log: makes startup backfill, daily runs and failures traceable.
CREATE TABLE IF NOT EXISTS schedule_runs (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  run_on TEXT NOT NULL,
  trigger TEXT NOT NULL,
  horizon_days INTEGER NOT NULL DEFAULT 30,
  generated INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  detail TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_schedule_runs ON schedule_runs(household_id, run_on);

CREATE INDEX IF NOT EXISTS idx_stmt_instrument_due ON credit_statements(household_id, due_on);
CREATE INDEX IF NOT EXISTS idx_debt_schedule_due ON debt_schedule_items(due_on, status);
CREATE INDEX IF NOT EXISTS idx_inst_schedule_due ON installment_schedule_items(due_on, status);
