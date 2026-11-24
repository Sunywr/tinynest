CREATE TABLE IF NOT EXISTS project_budgets (
  id TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES households(id),
  name TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  UNIQUE (household_id, name)
);
CREATE TABLE IF NOT EXISTS transaction_budget_links (
  transaction_id TEXT PRIMARY KEY REFERENCES transactions(id),
  budget_id TEXT NOT NULL REFERENCES project_budgets(id)
);
CREATE INDEX IF NOT EXISTS idx_transaction_budget ON transaction_budget_links(budget_id);
