ALTER TABLE receipt_jobs
  ADD COLUMN stage TEXT NOT NULL DEFAULT 'ocr';

CREATE TABLE receipt_ocr (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  receipt_id UUID NOT NULL REFERENCES receipts(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  status TEXT NOT NULL,
  raw_text TEXT,
  data_json JSONB,
  error TEXT,
  input_hash TEXT,
  run_version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_receipt_ocr_receipt_id ON receipt_ocr(receipt_id);
CREATE INDEX idx_receipt_ocr_created_at ON receipt_ocr(created_at);

CREATE TABLE receipt_suggestions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  receipt_id UUID NOT NULL REFERENCES receipts(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  status TEXT NOT NULL,
  prompt_json JSONB,
  raw_response JSONB,
  parsed_json JSONB,
  confidence NUMERIC,
  error TEXT,
  input_hash TEXT,
  run_version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_receipt_suggestions_receipt_id ON receipt_suggestions(receipt_id);
CREATE INDEX idx_receipt_suggestions_created_at ON receipt_suggestions(created_at);

CREATE TABLE draft_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  draft_id UUID NOT NULL REFERENCES draft_transactions(id) ON DELETE CASCADE,
  receipt_ocr_id UUID REFERENCES receipt_ocr(id),
  receipt_suggestion_id UUID REFERENCES receipt_suggestions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_draft_sources_draft_id ON draft_sources(draft_id);

CREATE TABLE tags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (entity_id, name)
);

CREATE TABLE receipt_tags (
  receipt_id UUID NOT NULL REFERENCES receipts(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (receipt_id, tag_id)
);

CREATE TABLE mileage_tags (
  mileage_id UUID NOT NULL REFERENCES mileage_logs(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (mileage_id, tag_id)
);

CREATE TABLE transaction_tags (
  transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (transaction_id, tag_id)
);

CREATE TABLE receipt_metadata (
  receipt_id UUID PRIMARY KEY REFERENCES receipts(id) ON DELETE CASCADE,
  data_json JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE mileage_metadata (
  mileage_id UUID PRIMARY KEY REFERENCES mileage_logs(id) ON DELETE CASCADE,
  data_json JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE audit_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  actor_user_id UUID REFERENCES users(id),
  object_type TEXT NOT NULL,
  object_id UUID,
  action TEXT NOT NULL,
  before_json JSONB,
  after_json JSONB,
  correlation_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_events_entity_id ON audit_events(entity_id);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);

CREATE TABLE processing_errors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  receipt_id UUID REFERENCES receipts(id) ON DELETE CASCADE,
  mileage_id UUID REFERENCES mileage_logs(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  error TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  resolution_note TEXT
);

CREATE INDEX idx_processing_errors_entity_id ON processing_errors(entity_id);
CREATE INDEX idx_processing_errors_created_at ON processing_errors(created_at);
