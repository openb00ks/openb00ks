# API (v0) - Draft

This is a draft endpoint list. An OpenAPI spec will be authored in `docs/openapi.yaml`.

## Health

- GET /healthz
- GET /readyz

## Setup

- GET /setup/status
- POST /setup

## Auth

- POST /auth/register
  - disabled by default for OSS/self-hosted mode
  - enable only when both API and web are configured for public registration (`ENABLE_PUBLIC_REGISTRATION=true` and `PUBLIC_ENABLE_REGISTRATION=true`)
  - returns `403 {"error":"REGISTRATION_DISABLED"}` when public signup is not enabled
- POST /auth/login
- POST /auth/refresh
- POST /auth/logout
- POST /auth/logout-all

## Tenant session

- GET /tenants
- POST /tenants/{id}/switch

Refresh returns a new access token and a rotated refresh token:

```
POST /auth/refresh
{ "refresh_token": "..." }
```

Logout revokes a refresh token:

```
POST /auth/logout
{ "refresh_token": "..." }
```

Logout all revokes every refresh token for the authenticated user:

```
POST /auth/logout-all
Authorization: Bearer <access token>
```

## Entities

- GET /entities
- POST /entities
  - fields: name, optional suggestion_context, optional template
- PATCH /entities/{id}
  - fields: optional name, optional suggestion_context
- DELETE /entities/{id}
- GET /entity-templates
  - public; lists account-set templates (key, name, account_count) for the entity-create picker. `POST /entities` and first-run `POST /setup` accept a `template` key to seed that starter chart of accounts (default: `basic`).

## Entity membership / roles

- GET /entities/{id}/members
- POST /entities/{id}/members
- PATCH /entities/{id}/members/{member_id}
- DELETE /entities/{id}/members/{member_id}

## Accounts

- GET /entities/{id}/accounts
  - ordered by chart-of-accounts `code` (nulls last), then name
- POST /entities/{id}/accounts
  - fields: name, type, optional code (chart-of-accounts number), optional role_tags[]
- PATCH /accounts/{id}
  - fields: name, type, optional code, optional role_tags[]
- DELETE /accounts/{id}

## Receipts

- POST /receipts (multipart upload)
  - fields: file, entity_id, optional total_cents, optional tags[], optional suggestion_context (legacy: context)
- GET /receipts/{id}
- GET /receipts/{id}/status
- PATCH /receipts/{id}/tags
  - receipt status: uploaded | queued | processing | ready_for_review | posted | needs_attention
  - includes OCR metadata + stored AI payload when available
  - includes error history for OCR/AI runs when present
- GET /receipts/{id}/ocr
  - returns latest OCR artifact + history
- POST /receipts/{id}/ocr/rerun
  - requeues OCR stage; invalidates downstream artifacts
- GET /receipts/{id}/suggestion
- POST /receipts/suggestions/batch
- Suggestion rows include optional token usage (prompt/completion/total) and cost_cents when available.
  - returns latest AI payload + parsed suggestion + history
- POST /receipts/{id}/suggestion/rerun
  - requeues suggestion stage; retains OCR artifact
- POST /receipts/{id}/draft/rerun
  - requeues draft generation only
- PATCH /receipts/{id}/vendor
  - fields: vendor_id — re-point a receipt's resolved vendor before posting; posting then trains that vendor's default account (see `receipt-pipeline.md`, vendor memoization)

## Bulk imports

- POST /imports (multipart or text)
  - fields: entity_id, file OR text, optional filename, optional content_type, optional suggestion_context (legacy: context)
- GET /imports?entity_id=&status=
- GET /imports/{id}
- GET /imports/{id}/rows
- PATCH /imports/{id}/rows/{row_index}
  - fields: account_id
- POST /imports/{id}/rows/{row_index}/post
  - posts one persisted import row as a balanced transaction using the mapped account and default Cash account
- POST /imports/{id}/rows/post-mapped
  - posts every mapped, unposted row and returns per-row results
- GET /imports/{id}/ocr
- POST /imports/{id}/ocr/rerun
- GET /imports/{id}/suggestion
- POST /imports/{id}/suggestion/rerun
- POST /imports/{id}/requeue

## Receipt processing

- Receipts are enqueued after upload and processed asynchronously.
- Processing performs OCR first (persisted), then runs rules + AI to produce suggestions.
- AI responses are always persisted (even on errors or low confidence).
- Processing produces suggestions and a draft transaction for review.

## Transactions

- POST /transactions
- GET /transactions?entity_id=&limit=

## Search

- GET /search?entity_id=&q=&kinds=&statuses=&account_ids=&tags=&start_date=&end_date=&limit=
  - searches the unified document search index using tenant + entity scope
  - indexed kinds currently include `transaction`, `receipt`, `import`, `account`, `statement`, and `mileage`
  - `q` is optional when filters are present; empty query searches all scoped indexed documents
  - `kinds`, `statuses`, `account_ids`, and `tags` are optional comma-separated filters
- POST /search/reindex?entity_id=
  - reindexes one entity's transactions, receipts, imports, accounts, statements, and mileage into the configured search provider
- GET /search/transactions?entity_id=&q=
  - searches the transaction-specific index using tenant + entity scope
- POST /search/transactions/reindex?entity_id=
  - reindexes one entity's posted transactions into the transaction-specific index

## Vendor rules

- GET /vendor-rules?entity_id=
- POST /vendor-rules
- PATCH /vendor-rules/{id}
- DELETE /vendor-rules/{id}

## Suggest

- POST /suggest
  - input: receipt_id, optional extracted text/context
  - read-only endpoint: returns the latest stored receipt suggestion summary when present; otherwise it may return a vendor-rule fallback based on existing receipt metadata and never queues or persists new suggestion data
  - output: receipt_id, suggestion_id, status, optional entity_id, optional account_id, confidence, optional explanation, optional raw_payload
  - returns `404 {"error":"NO_SUGGESTION"}` when neither a stored suggestion nor a read-only vendor-rule fallback is available

## System settings (admin)

- GET /settings/system
- PATCH /settings/system

## Exports

- GET /exports/transactions.csv?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
  - Includes source provenance columns for posted import rows when available.
- GET /exports/tax-pack.zip?entity_id=&year=YYYY
  - Alternative range form: `/exports/tax-pack.zip?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD`
  - Includes P&L, GL, transactions, mileage, import summary, exceptions, and a README/checklist.

## Reports

- GET /reports/general-ledger?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
- GET /reports/profit-loss?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
- GET /reports/balance-sheet?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
- GET /reports/mileage?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
- GET /reports/tax-readiness?entity_id=&year=YYYY
  - Alternative range form: `/reports/tax-readiness?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD`
  - Returns posted entry line count, import reconciliation summaries, and exceptions that should be resolved before tax export.

## Account statements

- GET /account-statements?entity_id=&account_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
- POST /account-statements
  - Required: `entity_id`, `account_id`, `period_start`, `period_end`, `starting_balance_cents`, `ending_balance_cents`
  - Optional: `source_receipt_id`, `status`, `notes`
  - Balances are signed book balances: bank/asset balances are usually positive; card/loan liability balances are usually negative.
- PATCH /account-statements/{id}
- POST /account-statements/{id}/reconcile
  - Sets status to `reconciled` only when the computed difference is zero and source import rows are posted; otherwise sets `needs_review`.

## Preferences

- GET /me/preferences
- PATCH /me/preferences

## Mileage

- GET /mileage?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD&limit=
- POST /mileage (optional suggestion_context, legacy: context)
- PATCH /mileage/{id} (optional suggestion_context, legacy: context)
- DELETE /mileage/{id}
- GET /exports/mileage.csv?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
  - mileage entries support tags/metadata for later search
  - mileage processing supports suggestion reruns without OCR

## Mileage reporting

- GET /reports/mileage?entity_id=&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD

## Mileage rates

- GET /mileage-rates
- GET /mileage-rates/{year}
- PUT /mileage-rates/{year}

## Admin migrations

- GET /migrations
- POST /migrations/up

## Admin users

- POST /users
- GET /users
- POST /users/{id}/mfa/reset

## Admin dashboards

- GET /admin/stats — usage/overview stats
- GET /admin/queue/jobs — queue state (pending/failed jobs)
- POST /admin/queue/jobs/{id}/requeue — requeue a job
- GET /admin/processing-errors — pipeline processing errors across receipts/imports
- POST /admin/processing-errors/{id}/resolve — mark a processing error resolved
