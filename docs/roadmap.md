# Roadmap

Goal: make Open B00KS reliable for local, self-hosted year-end books and tax-prep workpapers.

Status verified against the current API/web surface and implementation docs on 2026-07-23.

## Current tax-prep status

Open B00KS is far enough along to use locally as a disciplined bookkeeping workpaper tool:

- create tenants, users, entities, members, accounts, account role tags, entity fiscal-year settings, and entity home-use allocation settings
- import bank/card CSVs, persist row-level import data, map rows, detect duplicate candidates, and post mapped rows
- store statement starting/ending balances and reconcile imported activity by account and period
- capture receipts, process suggestions, review drafts, and post balanced journal entries
- track mileage and mileage rates
- generate reports and a tax pack with readiness blockers, review actions, home-use allocation data, account role tags, mileage, imports, P&L, GL, and transaction exports
- run with Taskfile locally, Docker Compose for OSS onboarding, or the generic Helm chart for Kubernetes packaging

It is not a tax-return generator. The output is a bookkeeping and preparer handoff bundle. Before filing, imported totals and statement balances still need to be reviewed, categories checked, and tax-specific items such as assets, loans, payroll, 1099s, sales tax, owner equity, and depreciation checked manually.

Legend:

- `[x]` complete enough for v0 flow
- `[~]` usable but still has important follow-up work
- `[ ]` still remaining

## Near term (tax-ready workflow)

- [x] Core auth + session stability for local use: self-host setup, login/logout, refresh cookies, local admin recovery, MFA/recovery flows, request logging, and stale browser-auth cleanup
- [x] Entities: create/select, default entity preference
- [x] Accounts: chart of accounts CRUD, account role tags, and tag export for tax-prep allocation review
- [x] Account-set templates: a default `basic` chart plus business templates (software startup, property management, short-term rental, small retailer), chosen at entity creation on both the entities page and first-run setup (`GET /entity-templates`)
- [x] Entity home-use allocation settings: office square-foot ratio plus phone and internet business-use ratios
- [x] Entity fiscal-year settings: defaults to Jan 1 and drives `year=` tax-prep date ranges
- [x] Receipt capture: upload + metadata + tags
- [x] Receipt processing pipeline: decomposed extract -> vendor-match -> classify -> build-entry (schema-bound, confidence-gated; `PIPELINE_MODE` = legacy `runSuggest` / `decomposed` / `decomposed-batch`), tiered PDF OCR (local text extraction, escalating to AI only when needed), and first-class vendor memoization (vendors table + alias ledger + per-vendor default account, Typesense `_vendors` retrieval, fail-open). See `receipt-pipeline.md`.
- [x] Review queue: status, confidence, retry
- [x] Draft editor: adjust entries, ensure balanced
- [x] Post transaction: receipt attachment + status update
- [x] Transactions list: filter by entity/date
- [x] Exports: transaction CSV, mileage CSV, reports, tax-readiness, and tax-pack ZIP
- [x] Bulk import (CSV): import common bank/card CSVs, persist rows, review parse errors, map rows, post mapped rows, and surface duplicate candidates
- [x] Audit trail for posting + edits (minimal)
- [x] Tax-prep checklist and preparer summary pages
- [x] Account statements: starting/ending balances, source import attachment, computed differences, reconciliation status, and statement CSV in tax pack
- [x] Tax-pack readiness blockers: unmapped imports, unposted rows, duplicate candidates, parse errors, missing receipt review, missing home-use allocation, account role-tag coverage, statement differences, unreconciled statements, and mileage-rate gaps
- [~] Basic error handling + empty states across UI: many flows covered; continue tightening as real local use exposes rough edges
- [~] Local tax-prep reliability: usable for local books; still needs statement coverage checks, more import presets, and accountant-style review polish

## Mid term

- [~] Vendor handling: **vendor memoization** is now the primary automatic matcher — the pipeline promotes raw receipt strings into first-class vendors (name + aliases + default account) that match and reuse over time. Legacy `vendor_rules` CRUD + suggestion use also remain (used by the legacy `runSuggest` path). Remaining: vendor-rules bulk import/export tooling, and consolidating `runSuggest` onto the vendor-memoization path.
- [x] Reports: P&L, Balance Sheet, GL
- [x] Mileage logging + export
- [~] Tags/metadata: tags/context stored and editable; broader tag-first filtering remains
- [x] Search and historical retrieval: Typesense-backed transaction indexing/search, unified search across transactions, receipts, imports, accounts, statements, mileage, and vendors, a dedicated `_vendors` retrieval collection for the pipeline, scoped candidate retrieval, reindex CLI/Taskfile/API paths, richer filters, leak guards, and a periodic ops-scheduler search-reconcile task (`SEARCH_RECONCILE_SECONDS`) that heals stale/missed index writes. Fail-open (degrades to DB when Typesense is off). Remaining: broader tag sync.
- [x] User management and roles UI: entity member management plus a system user admin UI (`/users` — create/list users, reset MFA) and entity-role assignment
- [~] Deployment packaging: Docker Compose and generic Helm chart exist; production values and environment-specific deployment config intentionally live outside this repo
- [~] Statement reconciliation: statement period records, math, source import attachment, and status are implemented; remaining work is coverage detection by fiscal year and richer reconciliation UX
- [x] Admin dashboards: usage stats, queue state with requeue, and processing-errors list/resolve (`/admin` UI + `/admin/*` API); operational health via the Prometheus `/metrics` endpoint (see `deployment.md`)

## Longer term

- [ ] Bank feed imports: OFX/QFX and richer bank-specific CSV presets
- [~] Advanced AI context + explainability: suggestion rationale + per-stage confidence are surfaced in the receipt review UI (persisted `ai_summary`), and the reviewer feedback loop is built — posting a corrected draft trains the vendor's default account and reassigns the raw-string alias, and a vendor can be re-pointed before posting. Remaining: richer account-policy context fed to the classifier.
- [ ] Multi-currency
- [ ] Mobile wrapper (Capacitor)
- [ ] Integrations (QuickBooks, Xero exports)
