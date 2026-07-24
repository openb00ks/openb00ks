CREATE TABLE import_rows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  receipt_id UUID NOT NULL REFERENCES receipts(id) ON DELETE CASCADE,
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  row_index INT NOT NULL,
  date DATE NOT NULL,
  vendor TEXT NOT NULL,
  memo TEXT,
  amount_cents BIGINT NOT NULL,
  direction TEXT NOT NULL,
  account_id UUID REFERENCES accounts(id),
  fingerprint TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'needs_review',
  transaction_id UUID REFERENCES transactions(id),
  raw_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (receipt_id, row_index)
);

CREATE INDEX idx_import_rows_receipt_id ON import_rows(receipt_id);
CREATE INDEX idx_import_rows_entity_id ON import_rows(entity_id);
CREATE INDEX idx_import_rows_fingerprint ON import_rows(entity_id, fingerprint);
CREATE INDEX idx_import_rows_status ON import_rows(entity_id, status);
