# Local development

## Overview

- API runs on Go.
- Web runs on SvelteKit dev server.
- Postgres is assumed to be running (local install or container).
- Receipts stored on local filesystem by default.
- Background worker processes receipts for OCR + AI/rules suggestions.
- Typesense is optional and can be used for fast search and historical suggestion retrieval.

## Env vars (initial)

- APP_ENV=dev
- DATABASE_URL=postgres://...
- JWT_SECRET=...
- REFRESH_TTL_SECONDS=...
- ENABLE_PUBLIC_REGISTRATION=false|true
- LOG_LEVEL=info|debug|warn|error
- LOG_FORMAT=json|text
- RECEIPT_STORAGE=local
- RECEIPT_LOCAL_DIR=./.data/receipts
- RECEIPT_MAX_BYTES=10485760
- AI_PROVIDER=openai|none
- OPENAI_API_KEY=...
- OPENAI_MODEL=gpt-5-nano
- AI_INPUT_CENTS_PER_1K_TOKENS=...
- AI_OUTPUT_CENTS_PER_1K_TOKENS=...
- SEARCH_PROVIDER=typesense|none
- TYPESENSE_URL=...
- TYPESENSE_API_KEY=...
- TYPESENSE_COLLECTION_PREFIX=openb00ks
- CORS_ALLOWED_ORIGINS=http://localhost:5177
- PUBLIC_ENABLE_REGISTRATION=false|true (web build-time flag; should mirror `ENABLE_PUBLIC_REGISTRATION`)
- OTEL_SERVICE_NAME=openb00ks-api
- OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
- OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf|grpc
- OTEL_EXPORTER_OTLP_INSECURE=true|false
- QUEUE_BACKEND=memory|db|redis (draft)

## Dev URL config

- OSS-first default keeps public registration off; set both `ENABLE_PUBLIC_REGISTRATION=true` and `PUBLIC_ENABLE_REGISTRATION=true` when running in SaaS mode and exposing `/register`.

## Database

- Taskfile remains the direct local development path.
- The API assumes the database is already running; if not, it should log a clear connection error.
- Apply migrations: `task migrate:up`
- Check migration version: `task migrate:version`

## Docker Compose target

Compose is available for OSS onboarding. It should not replace Taskfile workflows, but it makes the default local stack easy for contributors who expect one command.

Initial Compose services should be:

- Postgres
- Typesense
- API
- Worker
- Web

MinIO can be added later when S3-compatible receipt storage is the default local integration path. Until then, local filesystem receipt storage is simpler.

Run the Compose stack:

```text
task dev/compose/up
```

Stop it:

```text
task dev/compose/down
```

Compose maps:

- web: http://localhost:5177
- API: http://localhost:8181
- Postgres: localhost:5436
- Typesense: http://localhost:8108

## Running services

- Run API server: `task dev/api`
- Run background worker: `task dev/worker`
- Run the web app: `task dev`
- Run local Postgres container: `task dev/db`

## Local admin recovery

If setup is complete and you forgot the local admin password, reset or create an admin without wiping data:

```text
task dev/reset-admin EMAIL=admin@openb00ks.local PASSWORD=change-me TENANT_NAME="Default Tenant"
```

This is intended for self-hosted/dev recovery. It marks setup complete, ensures the user is an admin, and attaches the user to a tenant.
Use `ADMIN_DATABASE_URL=postgres://...` with the task only when you need to target a non-default local database.

## Queue + worker

- Receipt uploads enqueue a job for async processing.
- A background worker consumes the queue, runs OCR first (persisted), then AI/rules to generate suggestions.
- Receipt status reflects processing state for UI display.

## Search + AI suggestion flow

The local suggestion order should match production:

1. exact rules and vendor rules
2. historical retrieval from accepted transactions, using Typesense when enabled
3. AI ranking or tie-breaking when local evidence is weak
4. user review and explicit posting

When Typesense or AI are not configured, the worker should continue with the layers that are available.

## Search reindexing

Search is derived from Postgres. The unified document index includes transactions, receipts, imports, accounts, statements, and mileage; the transaction-specific index powers historical suggestion retrieval. Reindexing can be run more than one way:

- local or ops CLI: `task search/reindex-transactions`
- unified document index: `task search/reindex-documents`
- one entity only: `task search/reindex-transactions ENTITY_ID=<entity-id>`
- one entity only for unified search: `task search/reindex-documents ENTITY_ID=<entity-id>`
- one tenant only: `task search/reindex-transactions TENANT_ID=<tenant-id>`
- one tenant only for unified search: `task search/reindex-documents TENANT_ID=<tenant-id>`
- authenticated API path for entity admins: `POST /search/reindex?entity_id=<entity-id>`
- authenticated API path for entity admins: `POST /search/transactions/reindex?entity_id=<entity-id>`

The API path is intentionally entity-scoped. Broader tenant/all-entity reindexing is reserved for CLI/ops usage.

Typesense queries must include both `tenant_id` and `entity_id` filters. If either scope is missing, the search provider fails closed instead of querying broadly.
