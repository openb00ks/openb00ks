# Processing pipeline

Overview of how captured items (receipts, mileage, imports) move through the async worker.

## Goals

- Fast capture with minimal required input (entity + file or mileage details).
- Async processing with clear status for UI.
- Stage-scoped reruns without redoing all work.
- Durable storage of OCR/AI artifacts and errors.

## Pipeline stages

### Receipts

The receipt flow is a decomposed, schema-bound, confidence-gated AI pipeline — each stage does one
narrow task, validated in Go before anything posts. **`docs/receipt-pipeline.md` is the authoritative
description** (stages, per-stage schemas, gates, OCR provider, batch path). The coarse
`ocr → suggest → draft` flow this doc originally sketched has been replaced by that pipeline.

### Mileage

Mileage has no OCR/extraction step; it reuses the same queue and status model.

1. capture
   - input: entity_id, date, optional distance, optional tags[]
   - output: mileage record + queue job
2. suggest
   - input: user input, tag context, prior posted transactions
   - output: suggestion for categorization + metadata
3. draft/review

## Queue & status

Jobs carry a stage and a target object (receipt_id or mileage_id) so workers can filter and reruns
stay stage-scoped. See `docs/queue.md` for the queue model and `docs/architecture.md` for the
UI-facing receipt status values. Status should reflect the latest stage outcome and be derived from
persisted artifacts.

## Rerun semantics

Reruns are stage-scoped and invalidate only downstream artifacts:

- Rerun OCR:
  - invalidates suggest + draft artifacts
  - requeues OCR stage
- Rerun AI:
  - keeps OCR artifact
  - invalidates suggest + draft artifacts
  - requeues suggest stage
- Rerun draft:
  - keeps OCR + AI artifacts
  - regenerates entries only

## Artifact storage

- OCR result: raw text + structured fields + provider metadata
- AI payload: raw response + parsed suggestion + errors (if any)
- Draft: entries + explanation + source artifact versions

Storing these as append-only rows is what enables reruns and history. All stage runs (start/end,
inputs, outputs, errors) are logged; see `docs/audit.md` for audit and retention expectations.
