ALTER TABLE receipts
  ADD COLUMN kind TEXT NOT NULL DEFAULT 'receipt';

CREATE INDEX idx_receipts_kind ON receipts(kind);
