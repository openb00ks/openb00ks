DROP INDEX IF EXISTS idx_receipts_kind;

ALTER TABLE receipts
  DROP COLUMN IF EXISTS kind;
