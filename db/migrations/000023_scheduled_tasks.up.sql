-- Recurring operational tasks run by the ops-scheduler (cmd/ops-scheduler). One row per registered
-- task; the scheduler upserts a row for each task it knows about on startup (ON CONFLICT DO NOTHING),
-- so operator changes to interval/enabled persist. It is a general scheduling substrate — db-backup is
-- the first task; notifications, batch-AI enrichment, cleanup, etc. are added by registering a handler
-- and seeding nothing (the scheduler self-seeds). Multi-replica safe: a run claims its row with
-- FOR UPDATE SKIP LOCKED and holds a lease via running_since (a crashed run is reclaimed after the lease).
CREATE TABLE scheduled_tasks (
    name             TEXT PRIMARY KEY,
    interval_seconds INTEGER     NOT NULL CHECK (interval_seconds > 0),
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    running_since    TIMESTAMPTZ,           -- non-null while a run holds the lease
    last_run_at      TIMESTAMPTZ,
    last_status      TEXT,                  -- 'success' | 'error'
    last_error       TEXT,
    last_duration_ms BIGINT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The scheduler's hot path: "which enabled tasks are due (and not currently leased)?"
CREATE INDEX idx_scheduled_tasks_due ON scheduled_tasks (next_run_at) WHERE enabled;
