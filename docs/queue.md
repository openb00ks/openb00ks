# Receipt processing queue (v0)

## Goals

- Minimal infra for self-hosted users.
- No lost jobs; clear visibility into stuck items.
- Easy path to swap in Redis/SQS later for SaaS scale.

## v0 choice: Postgres-backed queue

- Use a `receipt_jobs` table to persist work items.
- Queue interface hides implementation details.
- Worker claims jobs with a visibility timeout.

## Job lifecycle

- queued -> processing -> done
- failed -> queued (retry) or dead (exceeded max_attempts)
- Jobs carry a stage (ocr/suggest/draft) to allow stage-scoped reruns.
- Processing includes OCR first, then AI/rules suggestions, then draft generation.
- Receipt status should surface progress for UI display (queued/processing/ready_for_review/needs_attention).
- API exposes job status history via `GET /receipts/{id}/status`.

## Resilience behavior

- Claim uses `locked_until` to avoid double processing.
- If a worker crashes, job becomes visible after timeout.
- Retry policy with exponential backoff up to `max_attempts`.
- UI can requeue failed/dead items manually.

## SaaS path

- Implement a `Queue` interface in code.
- Add alternative adapters (Redis/SQS) without changing domain logic.

## Suggested defaults

- visibility timeout: 5 minutes
- max attempts: 5
