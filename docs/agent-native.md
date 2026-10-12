# Agent-native direction

Status: proposal, 2026-10-12. Nothing here is built yet. This is the "why" behind the next phase of the
roadmap; once a part ships, update `roadmap.md` and the topic docs, and trim it from here.

## Summary

Open B00KS should become **a small, correct, multi-entity double-entry ledger that an AI agent operates
through an MCP server, with a web UI where a person reviews, approves and reports.**

Today the product is built the other way round: a capture app with its own AI pipeline (OCR → extract →
vendor-match → classify → build-entry). That pipeline works, but it answers a narrow question ("which
account does this receipt go to?"), while the expensive part of bookkeeping for a small owner is
everything around it: finding what's missing, reconciling against statements, untangling who paid what
from which account, and producing something an accountant can file from. A general agent the user already
has (Claude, ChatGPT, a local model) is good at that work. What it needs is a ledger it can trust and an
interface built for it.

The audience doesn't change: people who run a few small businesses, don't need QuickBooks, and shouldn't
pay $30–40 a month **per entity** or fake entities with tracking tags.

## Where this comes from

In October 2026 an agent (Claude Code) closed a household's 2025 books in Xero over a few days: two
property LLCs, a software LLC and a sole proprietorship. It went from "avoided for months" to "sent to the
accountant." The agent drove Xero through its API using small scripts. What made it work, and what got in
the way, is the spec for this direction.

### What worked

1. **The agent did the reasoning; the ledger did the bookkeeping.** The agent read bank statements, PDFs,
   screenshots, spreadsheets and texts the user paraphrased, decided what each item was, and asked the user
   short, specific questions ("was this check for the basement work or the cabin?"). The ledger's job was
   to enforce double entry and answer queries.
2. **Writes were safe to retry and easy to check.** Every posted batch carried a deterministic reference
   (`<SOURCE>-<date>-<amount>`), so re-running a script never duplicated anything. Larger changes were
   posted as drafts or in small batches and checked before the next one.
3. **Everything was checked against something outside the ledger:** bank statements, a lender's loan
   history, the trial balance, and a final scripted pass of 20 cross-checks between the deliverables and
   the ledger. Being right mattered far more than being fast.
4. **Decisions were written down.** A running notes file recorded each judgment call ("check 1243 was for
   Cabin C", "these two withdrawals paid invoice 1210"). That log is part of the books; the accountant and
   next year's close both need it.
5. **The output was a package, not a ledger.** P&L by entity and by property, balance sheets per entity
   with intercompany balances, a fixed-asset register by property, loan schedules, Schedule C worksheets
   with a home-office calculation, and a list of open questions for the preparer.

### What got in the way (Xero)

| Pain | What Open B00KS should do instead |
|---|---|
| Four businesses in one Xero org, separated by a tracking category, because a subscription per entity was too expensive | Already solved: many entities per tenant. Make it the headline. |
| Properties (parcels) inside one entity needed line-level tracking; Xero allows two tracking categories | **Dimensions on entry lines**, not just tags on transactions |
| The API couldn't update or delete fixed assets, create some settings, or set lock dates | Every operation an agent needs is in the API; protection comes from server rules (locks, approvals, audit), not from leaving operations out |
| Posting to a fixed-asset account auto-created ~170 junk draft assets | Fixed assets as a deliberate feature, created on purpose |
| Loan amortization schedules built by hand in scripts | Loans with generated schedules that split principal and interest |
| Owner contributions (personal cards paying business bills) and money one entity paid for another handled by hand | First-class owner contributions and draws, and due-to/due-from between entities in the same tenant |
| Rate limits, 30-minute tokens, a paid app tier just to connect | Personal access tokens; a local mode with no network at all |
| Edits blocked by the lock date with no way to lift it via the API | Period locks with an audited, explicit unlock |

## How Open B00KS compares today

Strong foundations to keep:

- Multi-tenant, multi-entity model (`tenants`, `entities`, `entity_users`).
- Balanced double-entry posting (`transactions` + `entries`, debits and credits in cents).
- CSV import with row persistence, mapping and duplicate detection; statement records and reconciliation.
- Mileage, entity tax settings (home-office ratio, phone and internet business use), account role tags.
- Reports (P&L, balance sheet, GL) and a tax pack with readiness blockers.
- Audit events.

Gaps against the workflow above:

- **No MCP server**, and no API credential other than a browser session (JWT + refresh cookie).
- **Tags live on transactions**, not entry lines, so one bill can't be split across properties.
- **No period locks, void or reversal.**
- **No fixed assets, depreciation or loan schedules.**
- **No contacts (payees) on transactions, no bills/payments, no owner-equity or intercompany helpers.**
- Posting logic is split between `internal/http` handlers and `internal/db/transactions.go`;
  `internal/domain` holds only types. A second front door (MCP) would duplicate the rules.
- Self-hosting needs Postgres, Typesense, object storage, a worker and an ops scheduler. That's a lot for
  someone running books for three LLCs at home.

## The shape of the product

```
  user's agent (Claude, ChatGPT, local model)        web UI (review, approve, reports)
              │  MCP (stdio or streamable HTTP)                │  REST
              ▼                                                ▼
        ┌──────────────────────── ledger service ────────────────────────┐
        │  posting rules · drafts & approvals · locks · dimensions ·     │
        │  idempotency · audit (actor = user | agent:<token>)           │
        └──────────────────────────────┬─────────────────────────────────┘
                                       ▼
                              Postgres (or SQLite)
```

### 1. One ledger service, two front doors

Move posting, drafting, approval, voiding and lock checks into a ledger service package (the natural home
is `internal/domain` or a new `internal/ledger`) that both the REST handlers and the MCP server call. The
HTTP handlers become thin adapters. This is worth doing even without MCP: it's where the invariants in
`architecture.md` should be enforced and tested.

### 2. The MCP server

- **stdio**: `openbooks mcp` runs next to the agent on the user's machine, talking to a local database or
  to a remote instance with a token. This is the easiest self-host story there is.
- **Streamable HTTP**: `/mcp` on the API server, for a hosted service and for agents that connect remotely.
- **Auth**: personal access tokens scoped to a tenant, optionally to entities, with `read` / `draft` /
  `post` scopes. OAuth can come later for hosted clients that require it.
- Use the official Go SDK (`github.com/modelcontextprotocol/go-sdk`) unless it proves unsuitable.
- Tools return compact JSON with stable IDs and amounts in cents plus a formatted string; agents make fewer
  mistakes when they don't have to parse display output.

### 3. Agents propose, people approve (configurable)

The current invariant ("nothing is posted without user confirmation") stays, and becomes workable at
agent speed:

- By default, agent writes create **drafts**. A person approves them in the web UI, one by one or as a
  batch ("approve these 37 drafts from import X").
- A token can be granted `post` scope by the user explicitly. It's still bounded by period locks, and it
  can't unlock a period.
- Every write is audited with its actor: `user:<id>` or `agent:<token id>`, plus the tool name and the
  agent-supplied reason.
- **Dry run** on every write tool: it returns the exact entries that would post, the balance check, and
  lock or duplicate warnings, and writes nothing.
- **Idempotency keys** on every write: posting the same key twice returns the original result. This is the
  `<SOURCE>-<date>-<amount>` pattern from the Xero close, enforced by the server.

**This changes a current invariant.** `AGENTS.md` and `CLAUDE.md` say AI never posts transactions or
creates data. Under this proposal an agent, acting with a token the user issued, can create drafts by
default and post only with a scope the user grants. If adopted, reword the invariant to: *"Nothing an
agent writes posts to the ledger without either a person's approval or a token scope a person explicitly
granted; every agent write is audited and reversible by void; suggestion endpoints stay read-only."*
Until that decision is made, Phase 1 ships with drafts only and no `post` scope.

### 4. MCP tool catalog (first cut)

Read:

- `list_entities`, `list_accounts`, `list_dimensions`
- `trial_balance(entity, as_of, by_dimension?)`
- `report(kind = pnl | balance_sheet | gl, entity, from, to, by_dimension?)`
- `search_transactions(entity, text?, amount?, date range?, account?, dimension?, contact?)`
- `get_transaction(id)` with lines, attachments, audit history
- `unreconciled(account, period)`: statement lines with no ledger match, and the reverse
- `period_status(entity)`: locks, open drafts, readiness blockers from the tax pack

Write (all support `dry_run` and `idempotency_key`):

- `post_transactions([...])` / `create_drafts([...])`: batch; each line has an account, an amount and
  optional dimensions
- `approve_drafts([...])` (needs `post` scope), `void_transaction(id, reason)` (posts a reversal; never
  deletes)
- `attach_document(transaction_id, file or url)`
- `import_statement(account, file, format)` and `match_statement_line(line, transaction)`
- `record_owner_contribution(entity, paid_from, expense lines)`: personal money paying business costs
- `record_intercompany(from_entity, to_entity, ...)`: posts both sides with due-to/due-from
- `create_fixed_asset(...)`, `update_fixed_asset(...)`, `dispose_fixed_asset(...)`
- `create_loan(...)`, `record_loan_payment(...)`: splits principal and interest from the schedule
- `add_note(entity, period, text, links)`: the decisions log
- `export_accountant_package(entity | tenant, year)`

Prompts/resources worth shipping with the server: a "year-end close" checklist, a "reconcile this
statement" playbook, and the entity's notes log, so any agent starts with the same method.

### 5. Ledger gaps, in order

1. **Line-level dimensions.** A typed key/value on each entry line (`property=300`, `project=…`), with
   reports filterable and groupable by it. Keep transaction tags for search; dimensions are for reporting.
2. **Period locks and void/reversal.** A lock date per entity; posting, editing or voiding inside it fails
   with a clear error. Unlocking is an audited, user-only action.
3. **Attachments on any transaction**, not just receipt-originated ones.
4. **Contacts** (payees and payers) on transactions. Needed for 1099 review and for "how much did we pay
   this contractor in total?"
5. **Owner equity and intercompany helpers** (above), plus due-to/due-from on per-entity balance sheets.
6. **Fixed assets and depreciation**: register by entity and dimension, placed-in-service date, method and
   life, a generated depreciation schedule, and journal entries posted from it. The preparer's schedule
   stays authoritative for tax; the register is the bookkeeping side.
7. **Loans**: terms, a generated amortization schedule, and payments that post principal and interest.
8. **Notes log**: per entity and period, linkable to transactions, included in the export.
9. **Bills and payments (minimal)**: only enough to record an invoice paid in several parts from several
   sources, which the Xero close needed for a contractor.

### 6. The accountant package is the product

The tax pack becomes the year-end deliverable, modelled on what the Xero close produced:

- P&L per entity and per dimension; balance sheet per entity with intercompany balances shown.
- Fixed-asset register with additions for the year; loan schedules with interest paid.
- Schedule C / Schedule E worksheets mapped from account role tags, including the home-office calculation
  (mortgage interest and property tax allowed in full; other home costs limited to profit and carried
  forward), mileage, and the de minimis purchases list.
- The notes log and an open-questions list for the preparer.
- Formats: a workbook (`.xlsx`) and a printable summary (PDF), plus the CSVs it already exports.

It stays a preparer handoff, not a return. The value is that a preparer can file from it without a
follow-up meeting.

### 7. Self-hosting that fits a single home server

- **One binary** (`openbooks serve`, `openbooks mcp`, `openbooks worker`), with the worker able to run
  in-process.
- **SQLite mode** for single-household installs; Postgres for hosted and multi-user.
- **Typesense, object storage and the receipt pipeline become optional.** Search falls back to the
  database (it already fails open); attachments fall back to the local filesystem.
- Docker Compose remains, but the default path is "download, run, point your agent at it."

### 8. What to defer

- New receipt-pipeline sophistication (more stages, more providers, batch tuning). Keep it working as an
  optional feature for people who want capture without an agent. Agents with vision cover most of it.
- The mobile wrapper (Capacitor) and further admin dashboards.
- Bank feeds via aggregators. Statement CSV/OFX import plus an agent covers the target user; aggregator
  costs would undercut cheap hosting.
- Multi-currency.

## Pricing implication

Price per **tenant (household or owner)**, not per entity. Someone with 2–4 entities pays once. The
self-hosted product is the full product (AGPL); a hosted version charges for hosting, backups and
convenience. Because reasoning runs in the user's own agent, hosting has no per-transaction inference cost
to pass on, which is what lets it be cheap.

## Phases

Each phase ends with something usable on real books.

**Phase 1: MCP over what exists.**
Ledger service extraction (REST handlers delegate to it), personal access tokens with scopes, MCP over
stdio and HTTP with the read tools, `create_drafts` / `post_transactions` with dry run and idempotency,
draft approval in the UI, audit actor. *Done when* an agent can import a statement, draft and post a
month's transactions for two entities, and produce a trial balance, with no direct database access.

**Phase 2: Ledger gaps.**
Line dimensions, period locks, void/reversal, attachments anywhere, contacts, owner contribution and
intercompany helpers, notes log. *Done when* the 2026 books for a multi-entity household (including
properties split by dimension) are kept in Open B00KS instead of Xero.

**Phase 3: Year-end.**
Fixed assets with depreciation schedules, loans with amortization, the accountant package
(workbook + PDF + notes + questions). *Done when* a 2026 year-end package goes to a preparer from Open
B00KS alone.

**Phase 4: Easy self-host.**
Single binary, SQLite mode, optional Typesense/S3/worker, release builds for Linux, macOS and Windows.
*Done when* a new user goes from download to an agent posting into their books in under ten minutes.

## Dogfooding plan

Run the household's 2026 books (four entities, two properties as dimensions) in Open B00KS through MCP,
migrated from Xero and spreadsheets during Phase 1–2. Every time the agent has to work around Open B00KS
the way it worked around Xero, that's a backlog item. The story it produces ("an agent closed four
businesses' books") is also the best pitch for the project.

## Open questions

- **Dimensions vs tags:** replace transaction tags with line dimensions, or keep both? (Proposal: keep tags
  for free-form search, add dimensions for reporting.)
- **Approval granularity:** is per-token `post` scope enough, or do users want limits (per amount, per
  account, per entity)?
- **SQLite:** how much of the current Postgres-specific SQL needs a second implementation, and is that
  worth it versus a bundled embedded Postgres?
- **The in-app pipeline's future:** keep it as an optional "no agent" path, or rebuild it as an MCP client
  that uses the same tools as everyone else?
- **Hosted auth for MCP:** which clients need OAuth on day one versus accepting a pasted token?
