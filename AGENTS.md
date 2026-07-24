# Agent context

## Product overview (v0)

Open B00KS is a capture-first bookkeeping tool for small operators. The core flow is: upload receipt -> suggest entity/account/payment -> user confirms -> post a balanced journal entry. AI is optional and never posts transactions; rule-based suggestions are always available.

## Core invariants

- Journal entries must balance: sum(debits) == sum(credits).
- Receipts are immutable once attached to a transaction.
- Suggestion endpoints are read-only and must not create data.

## Tech stack

- API: Go (REST, auth, domain logic).
- Web: SvelteKit + Tailwind (mobile-first).
- DB: Postgres (local via Docker).
- Receipt storage: S3-compatible object storage; local filesystem in dev.
- AI: BYOK provider interface; v0 supports OpenAI only.

## Draft API surface

- Health: GET /healthz, GET /readyz
- Auth: POST /auth/register, POST /auth/login
- Entities: GET/POST /entities, PATCH/DELETE /entities/{id}
- Entity members: GET/POST /entities/{id}/members, PATCH/DELETE /entities/{id}/members/{member_id}
- Accounts: GET/POST /entities/{id}/accounts, PATCH/DELETE /accounts/{id}
- Receipts: POST /receipts (multipart), GET /receipts/{id}
- Transactions: POST /transactions, GET /transactions?entity_id=&limit=
- Vendor rules: CRUD /vendor-rules
- Suggest: POST /suggest (receipt_id, optional text) -> suggested entity/account + confidence
- Export: GET /exports/transactions.csv?entity_id=

## Local dev (Taskfile)

- Install deps: `task install`
- Run web dev server: `task dev`
- Run API server (requires DB): `task dev/api`
- Run DB container: `task dev/db`
- Format: `task fmt`
- Tests: `task test`
- Build: `task build`

DB task uses Docker and defaults (from taskfiles/db.yaml):

- image: postgres:16
- name: openb00ks
- user: openb00ks_user
- password: openb00ks_dev
- port: 5436

## Env vars (draft)

- APP_ENV=dev
- DATABASE_URL=postgres://...
- JWT_SECRET=...
- RECEIPT_STORAGE=local
- RECEIPT_LOCAL_DIR=./.data/receipts
- RECEIPT_MAX_BYTES=10485760
- AI_PROVIDER=openai|none
- OPENAI_API_KEY=...
- OPENAI_MODEL=...

## Intended repo layout (draft)

- Go backend at repo root (go.mod at top)
- `/cmd`, `/internal`
- `/web` for SvelteKit app
- `/docs` for product/architecture docs
- `/build` for Docker/scripts

## Operational architecture

### Ops scheduler (`cmd/ops-scheduler`, `internal/ops`)

A small, self-contained recurring-task framework backed by the app's own Postgres — no external cron
or queue. Any background job that must run on a cadence registers as an `ops.Task` (a name, default
interval, and an idempotent `Run(ctx)`); `cmd/ops-scheduler` wires the enabled tasks and calls
`sched.Run`. Schedule state lives in the `scheduled_tasks` table (migration `000023`), which the
scheduler self-seeds on startup (`ON CONFLICT DO NOTHING`), so operator changes to `interval`/`enabled`
persist across restarts. The chart's existing migrate hook Job runs `migrate up`, so no extra chart
wiring is needed for the table.

- Multi-replica safe: a tick claims due rows with `SELECT ... FOR UPDATE SKIP LOCKED` and holds a
  lease via `running_since`; a crashed run is reclaimed after `SCHEDULER_LEASE_SECONDS`. One replica is
  the default (`replicas: 1` in the chart), but the lease makes >1 safe.
- Adding a task: implement `Run`, build an `ops.Task`, and `sched.Register(...)` it in `registerTasks`.
  Each capability should self-gate on its own config (an unconfigured task simply isn't registered), so
  a minimal self-host has nothing to break. This is a deliberate substrate for future work:
  notifications/digests, batch-AI enrichment, and cleanup.
- Metrics: task runs emit `openb00ks_ops_task_runs_total` and `openb00ks_ops_task_duration_seconds`
  (labels `task`, `outcome`) on the OTEL metrics port (`METRICS_ADDR`, `:9090`, named `metrics`), which
  also serves `/healthz`.
- Config: `SCHEDULER_TICK_SECONDS` (30), `SCHEDULER_LEASE_SECONDS` (3600).

### db-backup task (`internal/ops/backup.go`)

The first ops task: `pg_dump` → gzip → S3/R2, streamed through a seekable temp file so large dumps
don't buffer in memory. Objects land at `<prefix>/<db-label>/<UTC-timestamp>.sql.gz`; retention prunes
to the newest N (timestamp keys sort chronologically). It self-gates on `BACKUP_S3_BUCKET` — unset and
the task isn't registered. Because it needs a version-matched `pg_dump`, the `ops-scheduler` image is
built on `postgres:${PG_MAJOR}-alpine` (`Dockerfile.ops-scheduler`) rather than `scratch`. The point:
a self-hoster gets automatic offsite DB backups with only R2 credentials — no external cron or infra.
Restore is the inverse: `gunzip -c dump.sql.gz | psql "$DATABASE_URL"`.

- Config: `BACKUP_S3_BUCKET`, `BACKUP_S3_ENDPOINT`, `BACKUP_S3_REGION` (`auto` for R2),
  `BACKUP_S3_ACCESS_KEY_ID` / `BACKUP_S3_SECRET_ACCESS_KEY` (from the app Secret),
  `BACKUP_S3_FORCE_PATH_STYLE` (true), `BACKUP_S3_PREFIX` (`backups`), `BACKUP_DB_LABEL` (`openbooks`),
  `BACKUP_RETENTION` (14), `BACKUP_INTERVAL_SECONDS` (86400).
- Chart: `opsScheduler.enabled` (default false) renders a `<fullname>-ops-scheduler` Deployment; the two
  access keys are optional `secretKeyRef`s, non-secret backup config goes in `opsScheduler.env`.

### Receipt object storage (`internal/storage/s3.go`)

Receipts store to an S3-compatible bucket (Cloudflare R2) when `RECEIPT_STORAGE=s3`; reads are served
via presigned URLs. Local filesystem (`RECEIPT_STORAGE=local`) remains the dev/default. Config:
`RECEIPT_S3_BUCKET` / `RECEIPT_S3_ENDPOINT` / `RECEIPT_S3_REGION` (`auto`) and the
`RECEIPT_S3_ACCESS_KEY_ID` / `RECEIPT_S3_SECRET_ACCESS_KEY` credentials from the app Secret. This is
independent of the db-backup bucket (separate creds, separate bucket).

### PLANNED — Typesense search + AI pipeline (Phase 2, not yet built)

Deploy Typesense as an optional bundled dependency of *this* Helm chart (self-host ease: one `helm
install`, no separate search infra). Index receipts (vendor / amount / date / status + OCR text) to
give global search across receipts and transactions, and reuse that retrieval to feed AI suggestions —
similar past receipts/transactions as few-shot context plus vendor-rule matching.

- **Critical invariant: fail OPEN if Typesense is unavailable.** Search degrades to off (or the
  rule-based path) and a request must never crash because the search backend is down. Indexing and
  query paths both treat Typesense as best-effort.
- **Dependency:** receipt full-text and AI-from-content need real OCR. Transcription now ships via the
  pluggable `Transcriber` (`none` + `llm-vision`, see `docs/receipt-pipeline.md`); the remaining Phase 2
  work is the Typesense index itself, which stays fail-open.

### Receipt → journal-entry AI pipeline (Phase 3, BUILT)

Full design + implementation status: **`docs/receipt-pipeline.md`**. Replaces the coarse
`ocr → suggest → draft` flow with a decomposed, schema-bound, confidence-gated pipeline where each
AI call does ONE narrow task (extract fields / classify to one GL
account / adjudicate a vendor shortlist) at temperature 0 against a strict JSON schema, with a
deterministic Go validator after it (debit=credit, total=Σ line items) and a hard confidence gate →
`pending_approval` before anything posts. Selected via `PIPELINE_MODE` (`""` legacy single-call,
`decomposed` synchronous, `decomposed-batch` async); shared request builders + gates live in
`internal/pipeline`. Batch stages run as ops-scheduler `submit`/`collect` tasks. OCR is a pluggable
`Transcriber` — `none` + `llm-vision` shipped; `tesseract`/`textract` and Typesense-backed vendor
retrieval are deferred.

## Doc sources

- `docs/architecture.md`
- `docs/api.md`
- `docs/ai.md`
- `docs/local-dev.md`
- `docs/mobile.md`
- `docs/structure.md`
- `Taskfile.yaml`, `taskfiles/*.yaml`
