CREATE TABLE draft_transactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  receipt_id UUID NOT NULL REFERENCES receipts(id) ON DELETE CASCADE,
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  date DATE NOT NULL,
  memo TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_draft_transactions_receipt_unique ON draft_transactions(receipt_id);

CREATE TABLE draft_entries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  draft_transaction_id UUID NOT NULL REFERENCES draft_transactions(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id),
  debit_cents BIGINT NOT NULL DEFAULT 0,
  credit_cents BIGINT NOT NULL DEFAULT 0,
  CHECK (debit_cents >= 0 AND credit_cents >= 0),
  CHECK ((debit_cents = 0) <> (credit_cents = 0))
);

-- receipt_id unique index defined above
CREATE INDEX idx_draft_entries_draft_transaction_id ON draft_entries(draft_transaction_id);
