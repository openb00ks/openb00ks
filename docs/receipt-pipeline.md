# Receipt → Journal-Entry AI Pipeline (Phase 3)

Status: **BUILT.** The decomposed, schema-bound, confidence-gated pipeline described here is implemented
and replaces the coarse `ocr → suggest → draft` flow (where `ocr` was a stub and `suggest` was a single
broad AI call). It turns an uploaded receipt into a suggested, balanced journal entry with high accuracy.

## Implementation status (what's built vs. this design)

Selected via `PIPELINE_MODE`: `""` (legacy single-call `runSuggest`), `decomposed` (synchronous, worker
queue), `decomposed-batch` (async via the OpenAI Batch API + ops-scheduler). Both decomposed modes share
the same request builders + gates (`internal/pipeline`).

Built as specified, with refinements over the original sketch below:
- **Vendor memoization is a first-class `vendors` table** (`entity_id`, `name`, `normalized_name`,
  `match_pattern`, `tax_id`, `website`, `default_account_id`, `receipt_count`, `last_seen`) **plus a
  `vendor_aliases` ledger** — every raw receipt string that resolves to a vendor is recorded (normalized,
  `UNIQUE(entity_id, normalized)`), so matching accuracy compounds as receipts accrue. `default_account_id` is what lets a matched vendor skip the classify
  stage.
- **The vendor-match stage can recommend a *new* vendor** (`proposed_vendor`), returning enough to create
  one (clean name + match pattern + optional tax id / website) — not just match-or-fail. See the schema
  below.
- **Candidate retrieval is Typesense-first, fail-open.** A dedicated **`_vendors`** Typesense collection
  serves the machine-matching path (see *Vendor retrieval & search* below); when search is unavailable,
  errors, or returns nothing, retrieval falls back to **pure-Go trigram ranking** over the DB
  (`pipeline.RankVendorCandidates`; deterministic exact match first) — so search never gates correctness.

**Deferred:** `tesseract`/`textract` OCR providers — the `Transcriber` abstraction ships with `none` +
`llm-vision`.

### Vendor retrieval & search

Two Typesense collections, dual-indexed on write (mirroring the `_transactions`/`_documents` split):
- **`_documents`** — the polymorphic human **global search** index (transactions, receipts, accounts,
  statements, mileage, and vendors), surfaced by the `/search` UI page.
- **`_vendors`** — the dedicated **machine retrieval** index for the pipeline, carrying the full matching
  payload (`name`, `aliases[]`, `match_pattern`, `tax_id`, `default_account_id`, `receipt_count`,
  `last_seen_unix`; `token_separators: ["*","#"]` to split payment-processor noise). A hit builds a
  candidate ref with **no DB round-trip**. Typesense hits are used as-ranked (re-ranking would drop
  alias-only matches); the deterministic exact-normalized match is always folded in as a safety net.

Everything is fail-open and off by default (`SEARCH_PROVIDER=none`). Consistency is maintained three ways:
index-on-write (pipeline resolution + the vendors REST API keep both collections current), collection
auto-ensure at API/worker startup, and a periodic **`search-reconcile`** ops task (`SEARCH_RECONCILE_SECONDS`,
default 6h) that fully reindexes every entity as a drift healer.

A **`/vendors`** UI page (Books → Vendors) manages the first-class vendors the pipeline learns (name,
match pattern, default account, tax id/website, "seen N×"), and the global-search page filters vendors too.

## Thesis: accuracy is enforced by structure, not one big prompt

The principle: **decompose the work into
many stages; each AI call asks for exactly ONE narrow thing against a strict JSON schema at temperature
0, and a deterministic Go validator runs immediately after it.** A receipt is a row in a state machine
that advances one stage at a time. Bookkeeping is unusually friendly to this because **debit = credit**
and **total = Σ line items** are free correctness checks every AI output must pass.

Core design rules:
1. One narrow ask per AI call; per-stage `strict:true` JSON schema, temperature 0.
2. Deterministic wherever a model adds no accuracy (OCR transcription arithmetic, balancing, exact
   vendor/tax-id match, DB writes).
3. Retrieve, then let the model **judge** — never **recall**. Hand it the chart of accounts (enum) and a
   fuzzy-matched vendor shortlist; it picks, it doesn't invent.
4. Classify into a fixed enum (an existing GL account code), never free text. Off-list ⇒ review.
5. Evidence-only extraction: null/omit unknowns, never guess a tax amount or PO number.
6. Self-reported confidence + a hard gate → `pending_approval` **before** posting. An entry only posts
   when every stage cleared its gate (or a human approved).
7. Validate/repair each stage's output in Go (balance, totals, account exists, date sanity).
8. Memoize learned mappings: one AI call per **novel** vendor ever; a `vendors` row (with a default
   account) + an accumulating `vendor_aliases` ledger make future receipts from it deterministic —
   matched by candidate retrieval, and skipping classify.
9. Batch API for latency-tolerant runs (~50% cheaper); split `submit` / `collect` tasks on a schedule.
10. Model tiers by task shape: cheap structured model for extract/classify, premium only for judgment.
11. A frozen-prompt benchmark: the same request builder feeds prod and the eval harness, so per-stage
    accuracy regressions surface before shipping.

## What already exists (reuse, don't rebuild)

- **Job queue** (`internal/queue`, `receipt_jobs` table): `Enqueue`/`Claim` with `FOR UPDATE SKIP LOCKED`,
  per-stage claims, attempts/retry. Stages today: `ocr`, `suggest`, `draft`.
- **AI driver**: `internal/suggest` + `internal/aiconfig` wrap the shared `spectrum-labs-tech/ai` library —
  which already has the OpenAI **Batch API**, structured (JSON-schema) output, retries, and token/cost
  accounting. Per-tenant AI config via `aiconfig.Resolver`.
- **Ops scheduler** (`cmd/ops-scheduler`, `internal/ops`): the recurring-task substrate for the
  batch `*_submit` / `*_collect` tasks.
- **DB stores**: `receipts`, `receipt_ocr`, `receipt_suggestions`, `drafts`, `import_rows`, `vendor_rules`,
  `vendors`, `vendor_aliases`, `receipt_pipeline_state`, `ai_batch_jobs`, `accounts`, `entities`,
  `receipt_metadata`, `processing_errors`.
- **Object storage** (`internal/storage`): receipt images in R2 (presigned reads).
- **Search** (`internal/search`, Typesense): a `_vendors` collection for pipeline vendor retrieval + a
  `_documents` collection for global search, both fail-open (the pipeline falls back to Go trigram ranking
  and never requires Typesense). See *Vendor retrieval & search* above.

## Target pipeline

A receipt row advances through these stages. **[AI]** = one narrow model call; **[det]** = pure Go.
Batch stages run through the ops-scheduler (`submit`/`collect`); the interactive "scan & confirm now"
path runs the same stages synchronously through the worker queue.

| # | Stage | Kind | The one ask / job | Gate |
|---|-------|------|-------------------|------|
| 1 | **transcribe** (OCR) | [AI-vision] or [det] | image → raw text (transcription only) | text non-empty |
| 2 | **extract** | [AI batch] | text → `{vendor, date, currency, subtotal, tax, total, line_items[], conf}` | conf ≥ 0.75; total = subtotal+tax (±rounding) |
| 3 | **vendor-match** | [det]→[AI] | fuzzy shortlist → adjudicate to one `vendor_id` or "new" | conf ≥ 0.85; memoize alias |
| 4 | **classify-account** | [AI batch] | pick ONE `account_code` from the entity's chart of accounts | conf ≥ 0.80; account must exist |
| 5 | **build-entry** | [det] | assemble a balanced draft journal entry | debit = credit; total = Σ lines |
| 6 | **gate** | [det] | route by confidence / validation | any gate fail → `pending_approval` |
| 7 | **approve** | human / rule | operator confirms (or a vendor auto-post rule fires) | — |

Notes:
- **Stage 1 (transcribe)** is deliberately *just transcription*, not
  extraction — smaller input downstream, less hallucination surface, debuggable. OCR provider is a
  pluggable abstraction (below).
- **Stage 3 vendor-match** runs a deterministic exact match (normalized name) first;
  only a miss calls the model, which either adjudicates a **pre-retrieved** shortlist (Typesense `_vendors`
  retrieval, fail-open to Go trigram over the entity's `vendors`) to one `vendor_id`, **or recommends a new
  vendor** (`proposed_vendor`). A matched vendor carrying a **default account** skips stage 4 entirely; a
  new-vendor recommendation is upserted into `vendors` (default = the classified account). Either way the
  raw receipt string is recorded in `vendor_aliases`, so the vendor matches its own messy variants next
  time.
- **Stage 4** classifies into the entity's actual GL codes (enum from `accounts`); an off-list or
  low-confidence pick → review, never persisted as free text.
- **Stage 5** is pure arithmetic + double-entry construction — no model. It is also the **validator**:
  if the model's numbers don't balance, the row is parked, not posted.

## State machine

Reuse `receipt_jobs` for stage advancement; add richer **receipt statuses** (today: `processing`,
`needs_attention`, `ready_for_review`). Target receipt lifecycle:

```
uploaded → transcribing → extracting → matching_vendor → classifying → building → pending_approval → posted
                                    ↘ (any gate fail / low confidence) ↘ needs_review ──(human)──> posted
                                    ↘ (unreadable / not a receipt)      ↘ rejected
```

`JobStage` gains `transcribe`, `extract`, `vendor_match`, `classify`, `build` (superseding the coarse
`suggest`; keep `ocr`/`suggest`/`draft` as deprecated aliases during migration). Each stage, on success,
enqueues the next and resets that job's attempt budget (per-stage fresh retries). Batch stages
carry a batch-job id (new `ai_batch_jobs` / `ai_batch_items` tables) so `collect` reconciles
results by `custom_id`.

## Per-stage JSON schemas (concrete)

All schemas: `additionalProperties:false`, every field `required`, `strict:true`, temperature 0. Each
stage owns a single request builder shared by prod **and** the benchmark (frozen-prompt rule).

**extract** (structured/nano model):
```jsonc
{
  "vendor_name": "string|null",
  "date": "string|null",              // ISO 8601; null if not legible
  "currency": "string|null",          // ISO 4217
  "subtotal_cents": "integer|null",
  "tax_cents": "integer|null",
  "total_cents": "integer|null",
  "line_items": [                      // omit entirely if not itemized
    { "description": "string", "quantity": "number|null", "amount_cents": "integer" }
  ],
  "confidence": "number"               // 0.0–1.0, honest; ambiguous receipts score low
}
```
Prompt invariants: "Extract only what is present in the text. If a field is not legible, return null —
never guess. Amounts in integer cents. Do not compute totals you can't see."

**classify-account** (structured/nano): system prompt appends the entity's live account list
(`code — name`).
```jsonc
{ "account_code": "string", "confidence": "number", "reason": "string" }
```
Gate: `account_code` must be in the offered set (else item-failure → review), `confidence ≥ 0.80`.

**vendor-match** (structured/nano): only invoked on a deterministic miss; input = the raw vendor string
+ ≤8 candidate vendors (id, name, tax_id), retrieved from the `_vendors` Typesense collection (fail-open
to Go trigram similarity over the DB).
```jsonc
{
  "vendor_id": "string|null",          // set iff matching an offered candidate
  "is_new_vendor": "boolean",
  "proposed_vendor": {                 // set iff is_new_vendor; else null
    "name": "string",                  // cleaned canonical name ("SQ *BLUE BOTTLE" → "Blue Bottle Coffee")
    "match_pattern": "string",         // distinctive uppercase substring for future auto-match
    "tax_id": "string|null",
    "website": "string|null"
  },
  "confidence": "number",
  "reason": "string"
}
```
Gate: `confidence ≥ 0.85`; exactly one of `vendor_id` / `proposed_vendor` set; a returned `vendor_id` is
re-validated against the offered set before persist. A match with a default account skips classify; a
confident `proposed_vendor` ⇒ upsert a `vendors` row (default account = the classified account). Every
resolution records the raw string in `vendor_aliases` and refreshes the vendor's `_vendors` document.

## OCR provider decision

OCR is a pluggable provider: `OCR_PROVIDER=none|llm-vision`. **Only `none` and `llm-vision` are implemented
today** — the `tesseract`/`textract` bullets below remain design intent, not built. PDFs additionally run a
tiered, local-first path before any AI (see "Tiered PDF OCR (built)" below), so a text-layer PDF is
transcribed with no model call at all.

- **`llm-vision` (recommended default when AI is enabled):** send the image to a vision-capable model via
  the existing `spectrum-labs-tech/ai` driver, prompted to **transcribe only** (not extract). Best
  accuracy on messy receipts, one credential (the OpenAI key the app already uses), and it rides the
  same Batch API. Cost is per-image but small; fits the "scan nightly in a batch" model.
- **`tesseract` (fully self-contained option):** bundle Tesseract in the worker/ops image and shell to
  it — zero external API, the OSS-purist choice, but weaker on skewed/low-contrast receipts. Good default
  for a self-hoster who wants no cloud AI at all.
- **`textract` (optional, AWS users):** Textract `AnalyzeExpense` is purpose-built for receipts; keep as
  an opt-in provider, but it's a managed API (not self-hostable), so it's never the default.
- **`none`:** no transcription; the pipeline parks the receipt at `needs_attention` for manual entry.

### Tiered PDF OCR (built)

A PDF is never sent to the vision model as an image (OpenAI rejects `application/pdf` on the image input),
so the worker routes PDFs through a ladder before any AI:

1. **Tier 1 — local, no AI:** extract the embedded text layer with a pure-Go reader (`internal/ocr/pdf.go`,
   `ledongthuc/pdf`). Digital receipts/invoices carry a real text layer, so this transcribes them for free
   and deterministically; a `SufficientText` gate (length + digits) decides whether to trust it.
2. **Tier 2 — AI escalation:** only when tier-1 text is empty/too thin (a scanned/image-only PDF) is the PDF
   sent to the model as a **file input** (base64), which reads it directly (`internal/ocr/pdf_ai.go`).

Images still go straight to `llm-vision`; a local `tesseract` tier for images (so photos are AI-free too) is
the planned next step below.

Recommendation: implement the `Transcriber` interface with `llm-vision` + `tesseract`; default to
`llm-vision` when an AI provider is configured, else `tesseract`, else `none`. Keep transcription and
extraction as **separate stages** regardless of provider (accuracy + debuggability), even though a vision
model *could* do both at once.

## Scheduler integration (batch path)

Each batch AI stage becomes two ops-scheduler tasks (a `submit`/`collect` split):
- `pipeline-extract-submit` / `-collect`, `pipeline-classify-submit` / `-collect`, etc. `submit` claims
  `_pending` receipts, builds one OpenAI batch (JSONL, 24h window, `custom_id = <stage>-<receipt_id>`),
  records an `ai_batch_jobs` row; `collect` polls and applies results, advancing the stage.
- A **stuck-reset** guard at the top of each `submit` resets rows whose batch is `failed`/`expired` or
  older than 26h (key on `submitted_at`, never `updated_at`).
- The **synchronous** path (interactive upload) runs the same request builders inline via the worker
  queue for immediate feedback; only latency-tolerant/bulk runs use batch.

## Confidence gates & human review

`pending_approval` / `needs_review` is a non-terminal parking state reached from every gate and every
validation failure (unbalanced entry, off-list account, low-confidence vendor, illegible total). A
receipt only reaches `posted` when **all** stages cleared their gate **or** a human approved — never
auto-posted from review. Optional per-vendor **auto-post rules** (extend `vendor_rules`) let a trusted
vendor+account mapping post straight through once confidence is high — memoization applied to the
whole entry.

## Deterministic validators (the free correctness checks)

Run in Go after the model, before any write:
- `total_cents == subtotal_cents + tax_cents` (± rounding tolerance).
- If line items present: `Σ line_items.amount == subtotal` (or total).
- Built journal entry: **debit total == credit total** (double-entry invariant — the app already enforces
  balanced entries; the pipeline must produce one that passes).
- `date` within a sane window; `currency` valid ISO 4217; `account_code` exists for the entity.
Any failure → park at `needs_review` with a `processing_errors` row explaining which check failed.

## Evaluation harness (frozen-prompt benchmark)

The eval harness (`cmd/receipt-bench`): one request builder per stage shared by prod and the bench, a golden
test pinning the request body shape (so a driver bump fails loudly), and a labeled corpus of receipts
with known-correct `{vendor, date, total, account, entry}`. Replay the fixed request set on every
model/prompt change and diff **per-stage** accuracy against gold before shipping. Ground truth = operator-
approved historical entries (extract → stored fields, classify → chosen account, vendor-match → chosen
vendor). Keep raw prose/edge cases in an `aux` sidecar for a future LLM-judge.

## Model tiers & cost

- Cheap structured model (nano-class) for **extract**, **classify**, **vendor-match** (pattern/enum tasks).
- Premium model only where judgment is genuinely hard (ambiguous multi-vendor receipts, free-text memo
  generation). Reuse `suggest.EstimateCostCents` + the driver's usage recorder for per-stage cost
  accounting; the batch path already halves spend.

## Dependencies & phasing

1. ✅ **OCR provider** (`Transcriber`) — `none` + `llm-vision` shipped; `tesseract`/`textract` deferred.
2. ✅ **Decompose `suggest`** into `extract` + `classify` + `vendor-match` with per-stage schemas + gates
   (synchronous `decomposed` mode, worker queue).
3. ✅ **Vendor memoization** — first-class `vendors` table + `vendor_aliases` ledger, default-account
   carry, and new-vendor recommendation (`proposed_vendor`).
4. ✅ **Batch path** via the ops-scheduler (`decomposed-batch`; generic `internal/aibatch` framework,
   `ai_batch_jobs`, `receipt_pipeline_state`).
5. ✅ **Eval harness** (`cmd/receipt-bench`, `internal/eval`) — shared request builders, per-stage accuracy.
6. ✅ **Typesense retrieval + search** (fail-open) — dedicated `_vendors` retrieval collection + vendors in
   `_documents` global search, alias-driven matching, `search-reconcile` drift healer, and a `/vendors`
   management UI. Off by default (`SEARCH_PROVIDER=none`).
7. ⏳ **Remaining**: `tesseract`/`textract` OCR providers; a least-privilege Typesense key at deploy.

See also: `AGENTS.md` (Operational architecture) and the ops-scheduler (`cmd/ops-scheduler`).
