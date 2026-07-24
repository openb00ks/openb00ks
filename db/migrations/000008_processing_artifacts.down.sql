DROP TABLE IF EXISTS processing_errors;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS mileage_metadata;
DROP TABLE IF EXISTS receipt_metadata;
DROP TABLE IF EXISTS transaction_tags;
DROP TABLE IF EXISTS mileage_tags;
DROP TABLE IF EXISTS receipt_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS draft_sources;
DROP TABLE IF EXISTS receipt_suggestions;
DROP TABLE IF EXISTS receipt_ocr;

ALTER TABLE receipt_jobs
  DROP COLUMN IF EXISTS stage;
