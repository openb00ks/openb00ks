-- Async per-stage state for the batched receipt pipeline (PIPELINE_MODE=decomposed-batch). A receipt
-- advances extract → vendor → classify → done, each AI stage submitted via the generic aibatch
-- framework; build-entry (deterministic) runs inline when classify completes. This holds the
-- cross-stage intermediate outputs + the claim/lease so batches can process a stage across many
-- receipts. The synchronous path (PIPELINE_MODE=decomposed) does NOT use this table.
CREATE TABLE receipt_pipeline_state (
    receipt_id    UUID PRIMARY KEY REFERENCES receipts (id) ON DELETE CASCADE,
    stage         TEXT NOT NULL DEFAULT 'extract'
                      CHECK (stage IN ('extract', 'vendor', 'classify', 'done', 'review')),
    status        TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'in_flight')),
    extract_json  JSONB,
    vendor_json   JSONB,
    classify_json JSONB,
    failed_stage  TEXT,
    issues        TEXT,
    running_since TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The batch Gather's hot path: claim pending receipts at a given stage.
CREATE INDEX receipt_pipeline_state_claim_idx ON receipt_pipeline_state (stage) WHERE status = 'pending';
