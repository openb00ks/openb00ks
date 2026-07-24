-- Generic asynchronous AI batch tracking (internal/aibatch). A "kind" is any registered batch AI
-- operation (receipt pipeline stages, and future needs) — deliberately NOT a constrained enum, so
-- adding an operation needs no schema change. ref_id is the domain entity (e.g. a receipt id), kept
-- generic TEXT so the framework isn't tied to one table.
CREATE TABLE ai_batch_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              TEXT        NOT NULL,
    provider          TEXT        NOT NULL,
    model             TEXT        NOT NULL DEFAULT '',
    provider_batch_id TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'submitted'
                          CHECK (status IN ('submitted', 'completed', 'failed', 'expired')),
    item_count        INTEGER     NOT NULL DEFAULT 0,
    submitted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ,
    last_error        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The collector's hot path: open jobs, oldest first (also used by the stuck-reset).
CREATE INDEX ai_batch_jobs_open_idx ON ai_batch_jobs (submitted_at) WHERE status = 'submitted';
CREATE INDEX ai_batch_jobs_kind_status_idx ON ai_batch_jobs (kind, status);

CREATE TABLE ai_batch_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_job_id UUID        NOT NULL REFERENCES ai_batch_jobs (id) ON DELETE CASCADE,
    custom_id    TEXT        NOT NULL, -- reconciliation key returned by the provider
    ref_id       TEXT        NOT NULL, -- domain entity id (e.g. receipt id)
    status       TEXT        NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'applied', 'failed')),
    result       TEXT,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (batch_job_id, custom_id)
);

CREATE INDEX ai_batch_items_job_idx ON ai_batch_items (batch_job_id);
