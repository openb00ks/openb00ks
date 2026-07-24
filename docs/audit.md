# Audit logging (v0 draft)

## Goals

- Preserve a durable history of user input changes, suggestion results, and posting actions.
- Keep OCR/AI payloads and errors for reprocessing and support.
- Provide a clear trail for multi-user entities (capture vs confirm).

## Events to log

- Receipt created (actor, entity, file metadata, tags).
- OCR run started/completed/failed (inputs, provider, outputs, errors).
- AI suggestion run started/completed/failed (inputs, context, outputs, errors).
- Draft transaction created/updated (before/after diff).
- Transaction posted (final entries and accounts).
- Requeue/retry actions for receipt jobs.

## Data retention

- Store raw OCR text + structured fields.
- Store raw AI payloads and any parse errors.
- Keep error history even when a later rerun succeeds.
- Default retention for raw OCR/AI payloads: 6 months.
- Retain audit change events and posting snapshots indefinitely (or per tenant policy).

## Access

- Entity users can view audit entries for their entity.
- System admins can view cross-entity processing errors.
