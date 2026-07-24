# Architecture (v0)

## Goals

- Capture-first bookkeeping: receipt -> queue -> classify -> review -> post a double-entry journal entry.
- Mobile-first web UX; ready to be wrapped later by a mobile shell.
- Clean Go package boundaries with small interfaces and high test coverage.
- Clear deployment boundaries for local Taskfile, OSS Docker Compose, and Kubernetes/Helm.

## Non-goals (v0)

- Multi-currency.
- Full reporting suite (GL, P&L, Balance Sheet).
- Password reset and SSO.
- Native mobile app.

## Core workflow

1. User logs in.
2. Selects entity (or defaults to last-used).
3. Uploads receipt (photo or PDF) for fast capture (optional total amount + tags).
4. Receipt is stored as immutable and marked `uploaded`.
5. Receipt is enqueued for processing.
6. Async pipeline runs OCR (local text extraction for text-layer PDFs, else a vision model), then the decomposed stages — extract → vendor-match → classify → build-entry — each a schema-bound, confidence-gated AI call; the AI payload is stored even on errors. (A legacy single-call `runSuggest` path also exists, selected by `PIPELINE_MODE`.)
7. Receipt becomes `ready_for_review` with a draft transaction + suggestion metadata.
8. User confirms/edits and posts a journal entry.
9. Receipt is attached to the posted entry and marked `posted`.

## System boundaries

- API (Go): REST endpoints, auth, domain logic, storage, AI integration interface.
- Worker (Go): async receipt pipeline; reads from queue and writes suggestions/drafts.
- Web (SvelteKit): mobile-first UI; separate dev server.
- Storage:
  - Postgres for structured data.
  - Object storage for receipts (S3-compatible). Dev uses local filesystem.

## Domain model (draft)

- User
- Entity (business)
- EntityUser (membership + role)
- Account (chart of accounts)
- Receipt (immutable; object storage key + metadata + tags)
- OCRResult (linked to receipt; extracted text/fields + provider metadata)
- Suggestion (linked to receipt; AI/rule output + raw payload)
- DraftTransaction (editable; derived from suggestion)
- Transaction (journal entry header)
- Entry (journal entry lines)
- VendorRule (legacy vendor match -> suggested account/entity; used by the `runSuggest` path)
- Vendor + VendorAlias (vendor memoization: first-class normalized vendors with a per-vendor default account and a raw-string alias ledger; the decomposed pipeline learns and reuses these)
- AccountTemplate (embedded chart-of-accounts templates seeded at entity creation; see `chart-templates.md`)

## Invariants

- Transaction must balance: sum(debits) == sum(credits).
- Receipt is immutable once attached to a transaction.
- Suggest endpoint is read-only and never creates data.
- AI/rule suggestions are advisory only; nothing is posted without user confirmation.
- AI responses are always persisted (even on errors/low confidence) to avoid wasting paid inference.
- Minimum input for a draft is entity + receipt file; amount is optional and may be inferred.

## Receipt status lifecycle (v0)

- uploaded: file stored, awaiting processing
- queued: job enqueued for async processing
- processing: OCR + suggestion running
- ready_for_review: suggestion + draft ready
- posted: user-confirmed and journal entry created
- needs_attention: suggestion failed or confidence too low (optional)

## Permissions (v0)

- system_admin: platform-wide admin; manage users, entities, and system settings.
- entity_owner: full control within an entity (users, accounts, rules, transactions, exports).
- accountant: manage accounts, rules, transactions within assigned entities.
- user: create receipts/transactions and view data for assigned entities.

## AI integration (BYOK)

- v0 supports OpenAI only, behind a provider interface so other vendors can be added.
- Provider is optional; rule-based suggestions are always available.
- AI is used for receipt parsing/suggestion only; it never posts transactions.
- AI inputs should include user-entered fields, OCR output, vendor rules, and entity context for better confidence.
- Posted transactions are the source of truth; suggestions are advisory only.

## Receipt formats and limits (defaults)

- Allowed: image/jpeg, image/png, application/pdf.
- Max size: 10 MB (configurable).

## Export (v0)

- Required: CSV of journal entry lines.
- Optional: include receipt URL via a flag.

## Mileage tracking

- Standalone mileage logs per entity with optional receipt links.
- Mileage summary report by month with reimbursement estimates.
- CSV export for mileage logs.
- Mileage capture follows the same async suggestion workflow (minus OCR) for categorization and tags.
- Mileage entries require an entity; amount/distance can be inferred or entered later.

## Mileage rates

- The app ships US default mileage rates via a seed migration if the table is empty.
- Users can override rates in-app for their locale.

## Deployment

- OCI images are the deployable units for API, worker, and web.
- Docker Compose should support OSS local onboarding.
- A Helm chart should support Kubernetes deployment once runtime config stabilizes.
- Health endpoints: /healthz and /readyz.
- See `docs/deployment.md`.

## Future imports

- Bank statements or transaction logs should follow the same pipeline as receipts:
  upload -> parse -> suggest -> user review -> post.
- Goal: batch imports that reduce repetitive entry (e.g., monthly mortgage payments).
- Imports require review and confirmation in v0 (no auto-post).

## Audit logging

- Audit logging is required for input changes, suggestion results, and posting actions.
- OCR/AI runs are rerunnable; their inputs/outputs/errors should be recorded as part of audit history.
