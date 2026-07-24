# AI and suggestion integration (v0)

## Goals

- Allow BYOK (bring your own key) for intelligent features.
- v0 supports OpenAI through `github.com/spectrum-labs-tech/ai`, but the app should remain provider-ready.
- Use deterministic and historical evidence before spending tokens.
- Keep every suggestion reviewable, explainable, and non-posting.

## Suggestion order

Suggestions should be built as a pipeline, not as a direct AI call.

1. Exact and rule-based matching.
2. Historical candidate retrieval from accepted transactions.
3. AI ranking or tie-breaking over a compact candidate set.
4. User review and explicit posting.

Rules stay first because they are cheap, deterministic, and easy to audit. Historical matches come next because the user's accepted books are stronger evidence than a model's general knowledge. AI is used when the local evidence is weak, conflicting, or needs judgment.

> **Implemented pipeline.** The ordering above is the philosophy and the legacy single-call path
> (`PIPELINE_MODE=""`, `runSuggest`). The current, richer implementation is the **decomposed pipeline**
> (`PIPELINE_MODE=decomposed` / `decomposed-batch`): OCR → extract → vendor-match → classify → build-entry,
> each a small schema-bound, confidence-gated AI call. It adds **vendor memoization** (raw receipt strings
> become first-class vendors that match and reuse a default account), **explainability** (per-stage rationale
> + confidence persisted as `ai_summary` and shown in the review UI), and a **reviewer feedback loop**
> (posting a corrected draft trains the vendor's default account and reassigns its alias). The `runSuggest`
> and decomposed paths are not yet consolidated. Full design: **`receipt-pipeline.md`**.

## Historical retrieval

Typesense is the preferred retrieval layer when enabled. It should index accepted, posted bookkeeping data and return the most relevant prior matches for a receipt or import row.

Recommended indexed fields:

- tenant ID and entity ID
- transaction ID and source receipt/import row ID
- vendor or payee
- normalized memo and description
- receipt OCR text, when available
- account ID, account name, and account role tags
- amount and amount bucket
- transaction date and accepted/posting date
- posting status and review decision

Search should be scoped by tenant and entity by default. The worker can optionally widen the query when the entity is unknown, but cross-entity matches should be clearly marked in suggestion metadata.

Postgres `pg_trgm` remains a reasonable fallback for deployments that do not want a search service. Typesense is not required for correctness; it is an optional retrieval accelerator and product search feature.

## AI provider

The app uses `github.com/spectrum-labs-tech/ai` for provider-specific API calls. The local app owns prompt construction, domain validation, cost tracking, and audit records.

Current provider target:

- provider: `openai`
- driver package: `github.com/spectrum-labs-tech/ai/drivers/openai`
- config: `AI_PROVIDER=openai`, `OPENAI_API_KEY`, `OPENAI_MODEL`

The AI response must be schema-bound and validated against current entity data before it is stored as a suggestion. For account suggestions, an AI-returned account ID is only valid if that account belongs to the current entity.

## Behavior and guardrails

- AI features are optional and can be disabled.
- Rule-based suggestions are always available.
- AI never posts transactions; it only returns suggestions/metadata.
- AI runs asynchronously after receipt capture to avoid blocking uploads.
- AI responses are always persisted (even on errors/low confidence) to preserve paid inference.
- AI can be rerun with updated context or corrected images.
- AI should not invent accounts, entities, or tax classifications. It can choose from known candidates or return insufficient evidence.
- Suggestion records should preserve the evidence stack: winning rule, historical matches, AI request metadata, AI response metadata, confidence, and validation result.

## Config

- AI_PROVIDER=openai|none
- OPENAI_API_KEY=...
- OPENAI_MODEL=gpt-5-nano (for the decomposed pipeline prefer a stronger model such as `gpt-5-mini` — `nano` tends to fall below the extract/classify confidence gates and parks receipts)
- AI_INPUT_CENTS_PER_1K_TOKENS=...
- AI_OUTPUT_CENTS_PER_1K_TOKENS=...
- SEARCH_PROVIDER=typesense|none
- TYPESENSE_URL=...
- TYPESENSE_API_KEY=...
- TYPESENSE_COLLECTION_PREFIX=openb00ks

## Pipeline notes (v0)

- Receipt upload creates a draft record quickly.
- Background worker runs OCR/extraction and stores OCR output first.
- Worker then runs suggestion with context (user input, OCR, vendor rules, historical matches, entity/account context).
- Results (including raw AI payload/errors) populate a suggestion record and a draft transaction.
- Suggestion records should include token usage (prompt/completion/total) and computed cost (stored as cents) for later reporting.
- Account role tags and accepted prior transactions should be used to fetch a small set of reference candidates.
- Users review and post; posting creates the journal entry.

## Typesense as app search

The same Typesense index can power user-facing search for transactions, receipts, imports, accounts, and tags. That should be treated as a product feature layered on top of the suggestion retrieval index.

Initial search scope covers transactions, receipts, imports, accounts, statements, and mileage through the unified document index. Broader app-wide search can come later after index sync and permissions are reliable for more object types such as standalone tags, vendor rules, processing errors, and settings.

## Sync model

The first implementation can update the Typesense index synchronously after a transaction is accepted or changed. The production-friendly model is an outbox table consumed by the worker so index updates are retryable and observable.

Search index state is derived data. Postgres remains the source of truth.

## Future providers

- Anthropic, Gemini, others can be added by implementing the provider interface.
