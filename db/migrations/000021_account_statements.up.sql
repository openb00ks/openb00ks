ALTER TABLE entities
  ADD COLUMN fiscal_year_start_month INT NOT NULL DEFAULT 1,
  ADD COLUMN fiscal_year_start_day INT NOT NULL DEFAULT 1;

ALTER TABLE entities
  ADD CONSTRAINT entities_fiscal_year_start_month_check
    CHECK (fiscal_year_start_month >= 1 AND fiscal_year_start_month <= 12),
  ADD CONSTRAINT entities_fiscal_year_start_day_check
    CHECK (fiscal_year_start_day >= 1 AND fiscal_year_start_day <= 31);

CREATE TABLE account_statements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  source_receipt_id UUID REFERENCES receipts(id) ON DELETE SET NULL,
  period_start DATE NOT NULL,
  period_end DATE NOT NULL,
  starting_balance_cents BIGINT NOT NULL DEFAULT 0,
  ending_balance_cents BIGINT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'draft',
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (period_start <= period_end),
  CHECK (status IN ('draft', 'needs_review', 'reconciled', 'locked'))
);

CREATE INDEX idx_account_statements_entity_id ON account_statements(entity_id);
CREATE INDEX idx_account_statements_account_id ON account_statements(account_id);
CREATE INDEX idx_account_statements_period ON account_statements(entity_id, period_start, period_end);
CREATE UNIQUE INDEX idx_account_statements_source_receipt_id
  ON account_statements(source_receipt_id)
  WHERE source_receipt_id IS NOT NULL;
