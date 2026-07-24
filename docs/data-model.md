# Data model (v1 draft)

## Goals

- Describe the core tables and planned additions.
- Provide a stable target for migrations (avoid churn).
- Keep auditability, reruns, and tags first-class.

## Existing tables (current migrations)

- users
- entities
- entity_users
- accounts
- receipts
- receipt_jobs
- transactions
- entries
- vendor_rules
- vendors
- vendor_aliases
- drafts
- draft_entries
- preferences
- mileage
- mileage_rates
- account_statements
- scheduled_tasks (ops-scheduler recurring tasks)
- receipt_pipeline_state (decomposed-batch pipeline)

## Receipt-pipeline & supporting tables

> **Status:** originally drafted as "planned," these are now **all implemented** — the receipt pipeline
> (OCR → suggestions → drafts + provenance), tags, metadata, audit log, and processing errors are live
> (see `db/migrations/`). Detail below.

### OCR artifacts

Table: receipt_ocr

- id (uuid)
- receipt_id (uuid, fk receipts)
- provider (text)
- status (text: succeeded|failed)
- raw_text (text)
- data_json (jsonb) -- structured fields
- error (text)
- created_at (timestamptz)
- input_hash (text) -- for idempotency
- run_version (int) -- increment per receipt

### AI suggestions

Table: receipt_suggestions

- id (uuid)
- receipt_id (uuid, fk receipts)
- provider (text)
- model (text)
- status (text: succeeded|failed)
- prompt_json (jsonb)
- raw_response (jsonb)
- parsed_json (jsonb)
- confidence (numeric)
- error (text)
- created_at (timestamptz)
- input_hash (text)
- run_version (int)

### Vendor memoization

The pipeline promotes raw receipt vendor strings into first-class vendors that match and improve over time
(see `receipt-pipeline.md`).

Table: vendors

- id (uuid)
- entity_id (uuid, fk entities)
- name (text) -- clean canonical name
- normalized_name (text) -- exact-match key; unique (entity_id, normalized_name)
- match_pattern (text) -- memoization primitive for payment-processor noise (e.g. `SQ*`, `AMZN MKTP*`)
- tax_id (text), website (text)
- default_account_id (uuid, fk accounts) -- the expense account this vendor usually maps to
- receipt_count (int), last_seen (timestamptz) -- retrieval ranking signals
- created_at / updated_at (timestamptz)

Table: vendor_aliases -- the raw-string ledger

- vendor_id (uuid, fk vendors), entity_id (uuid, fk entities)
- raw_string (text) -- the messy string as printed on a receipt
- normalized (text) -- unique (entity_id, normalized): first-writer-wins on the pipeline path, REASSIGNED by a
  human vendor correction (`RecordConfirmed`)
- occurrences (int), last_seen (timestamptz)

### Receipt columns added by the pipeline

`receipts` gained: `resolved_vendor_id` (uuid, fk vendors, ON DELETE SET NULL), `resolved_vendor_raw` (text),
and `ai_summary` (jsonb) -- the display summary the review UI shows (vendor + account with confidence + reason,
persisted from both the sync and batch pipeline paths).

### Draft provenance

Table: draft_sources

- id (uuid)
- draft_id (uuid, fk drafts)
- receipt_ocr_id (uuid, fk receipt_ocr)
- receipt_suggestion_id (uuid, fk receipt_suggestions)
- created_at (timestamptz)

### Tags

Table: tags

- id (uuid)
- entity_id (uuid, fk entities)
- name (text)
- created_at (timestamptz)
- unique (entity_id, name)

Join tables:

- receipt_tags (receipt_id, tag_id)
- mileage_tags (mileage_id, tag_id)
- transaction_tags (transaction_id, tag_id)

### Metadata

Table: receipt_metadata

- receipt_id (uuid, pk, fk receipts)
- data_json (jsonb)

Table: mileage_metadata

- mileage_id (uuid, pk, fk mileage)
- data_json (jsonb)

### Audit log

Table: audit_events

- id (uuid)
- entity_id (uuid)
- actor_user_id (uuid)
- object_type (text)
- object_id (uuid)
- action (text)
- before_json (jsonb)
- after_json (jsonb)
- created_at (timestamptz)
- correlation_id (uuid)

### Processing errors (optional)

Table: processing_errors

- id (uuid)
- entity_id (uuid)
- receipt_id (uuid, nullable)
- mileage_id (uuid, nullable)
- stage (text)
- error (text)
- created_at (timestamptz)
- resolved_at (timestamptz, nullable)
- resolution_note (text, nullable)

## Notes

- Storing OCR and AI artifacts as append-only rows enables reruns and history.
- input_hash + run_version allow idempotent retries without losing history.
- Tags are entity-scoped and attached through join tables for flexibility.
