# open-b00ks

Open B00KS (pronounced "Open Books") is a capture-first bookkeeping tool for small operators who want their books recorded at the moment an expense happens - without SaaS rent or ERP overhead.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](./LICENSE)

## What it is (v0)

The core flow is: upload a receipt -> get a suggested entity/account/payment -> confirm -> post a balanced journal entry. AI is optional and never posts transactions; rule-based suggestions are always available.

## Product goals

- Mobile-first capture and confirmation.
- Clean, testable Go domain boundaries.
- Bring-your-own-key (BYOK) AI integration behind a provider interface.
- Support practical local development and self-hosted deployment paths.

## Tech stack

- API: Go (REST, auth, domain logic).
- Web: SvelteKit + Tailwind (mobile-first).
- DB: Postgres (local via Docker).
- Receipt storage: S3-compatible object storage; local filesystem in dev.
- AI: Provider interface, v0 supports OpenAI only.

## Core invariants

- Journal entries must balance: sum(debits) == sum(credits).
- Receipts are immutable once attached to a transaction.
- Suggestion endpoints are read-only and must not create data.

## Draft API surface (v0)

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

## Project structure (v0 draft)

```
/ (repo root)
  go.mod, go.sum
  Taskfile.yaml, taskfiles/       # task runner + split task files
  Dockerfile*                     # api / worker / ops-scheduler images
  compose.yaml                    # local full-stack dev
  /cmd                            # entrypoints
    /api                          # REST API server
    /worker                       # async job worker
    /ops-scheduler                # recurring tasks (backups, search reconcile)
    /receipt-bench                # AI-pipeline eval harness
  /internal                       # domain logic + adapters (not externally importable)
    /auth /domain /http /models   # core
    /db /storage /search          # adapters: Postgres, object storage, Typesense
    /queue /ops /pipeline         # async processing + receipt pipeline
    /aibatch /aiconfig /suggest /ocr /eval /vendormemo /receiptbatch  # AI
    /config /logging /telemetry /migrate /importer /templates /testutil
  /web                            # SvelteKit + Tailwind frontend
    /src /static
  /charts/open-b00ks              # Helm chart
  /db/migrations                  # SQL migrations
  /docs                           # architecture + design docs
```

## Local development

This repo uses Taskfile for common workflows.

- Install deps: `task install`
- Run web dev server: `task dev`
- Run API server (requires DB): `task dev/api`
- Run worker (requires DB): `task dev/worker`
- Run DB container: `task dev/db`
- Run full Compose stack: `task dev/compose/up`
- Reindex transaction search: `task search/reindex-transactions`
- Apply migrations: `task migrate:up`
- Format: `task fmt`
- Tests: `task test`
- Build: `task build`

## Environment variables (draft)

- APP_ENV=dev
- DATABASE_URL=postgres://...
- JWT_SECRET=...
- LOG_LEVEL=info|debug|warn|error
- LOG_FORMAT=json|text
- RECEIPT_STORAGE=local
- RECEIPT_LOCAL_DIR=./.data/receipts
- RECEIPT_MAX_BYTES=10485760
- AI_PROVIDER=openai|none
- OPENAI_API_KEY=...
- OPENAI_MODEL=...
- SEARCH_PROVIDER=typesense|none
- TYPESENSE_URL=...
- TYPESENSE_API_KEY=...
- TYPESENSE_COLLECTION_PREFIX=openb00ks
- CORS_ALLOWED_ORIGINS=http://localhost:5177
- METRICS_ADDR=:9090
- OTEL_SERVICE_NAME=openb00ks-api
- OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
- OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf|grpc
- OTEL_EXPORTER_OTLP_INSECURE=true|false

## Observability

- **Structured logging** — `log/slog` (JSON by default, `LOG_FORMAT`/`LOG_LEVEL`). Every request logs
  one `http_request` line (method, route, status, latency, bytes) and is **trace-correlated**
  (`trace_id`/`span_id`) so a log line links straight to its span.
- **Tracing** — OpenTelemetry spans for HTTP (`internal/http` middleware) and the database
  (`otelsql`), exported via OTLP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set (no-op otherwise).
- **Metrics** — Prometheus, pull-based, on a dedicated port (`METRICS_ADDR`, default `:9090`) at
  `/metrics`, kept off the public API surface. Emitted:
  - HTTP: `http_server_request_duration_seconds` (histogram by method/route-template/status) +
    `http_server_active_requests`. Request rate, error rate and p95 latency derive from these.
  - DB pool: `otelsql` connection stats (in-use/idle/wait) — registered against the meter provider.
  - Worker: `openb00ks_worker_job_duration_seconds` + `openb00ks_worker_jobs_processed_total` by
    stage (ocr/suggest/draft) and outcome (ack/fail).
  - Business events: `openb00ks_transactions_posted_total{source=direct|receipt}`,
    `openb00ks_receipts_uploaded_total`, `openb00ks_suggestions_served_total`. Suggestion accept rate
    = `rate(transactions_posted_total{source="receipt"}) / rate(suggestions_served_total)`.
  - Go runtime (GC, goroutines, memory) via the OTEL runtime instrumentation.

## Documentation

- `docs/architecture.md`: goals, invariants, system boundaries
- `docs/api.md`: draft endpoints
- `docs/ai.md`: AI provider notes
- `docs/deployment.md`: local runtime, Compose, and Kubernetes/Helm design
- `docs/data-model.md`: data model targets and planned tables
- `docs/pipeline.md`: async processing overview (queue, status, rerun semantics)
- `docs/receipt-pipeline.md`: decomposed receipt → journal-entry AI pipeline (authoritative)
- `docs/queue.md`: job queue and worker processing
- `docs/tags.md`: tags and metadata design
- `docs/audit.md`: audit logging goals and retention
- `docs/settings.md`: tenant and system settings, AI provider configuration
- `docs/multi-tenancy.md`: multi-tenancy model
- `docs/first-boot.md`: first-run bootstrap behavior
- `docs/tax-prep.md`: tax-preparation features
- `docs/testing.md`: testing guidelines and integration test constraints
- `docs/local-dev.md`: dev setup and env vars
- `docs/frontend.md`: frontend structure
- `docs/mobile.md`: mobile strategy (Capacitor)
- `docs/ux-design-plan.md`: UX and interface design principles
- `docs/structure.md`: repo layout
- `docs/chart-templates.md`: Helm chart templating notes
- `docs/roadmap.md`: roadmap
- `charts/open-b00ks`: generic Helm packaging, not environment deployment config

## Roadmap (non-goals in v0)

- Multi-currency.
- Full reporting suite (GL, P&L, Balance Sheet).
- Password reset and SSO.
- Native mobile app (Capacitor wrapper is planned later).

## License

Open B00KS is licensed under the **GNU Affero General Public License v3.0
(AGPL-3.0)** — see [`LICENSE`](./LICENSE). The AGPL's network-use clause means
anyone who runs a modified version as a network service must make their source
available under the same license.

Copyright (c) 2026 Spectrum Labs LLC.

Contributions are accepted under a Contributor License Agreement; see
[`CONTRIBUTING.md`](./CONTRIBUTING.md) and [`CLA.md`](./CLA.md).
