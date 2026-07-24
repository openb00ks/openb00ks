# UX and Design Rationale

Open B00KS is a double-entry bookkeeping application built around a single
guiding workflow:

```
capture -> review -> confirm -> post
```

This document describes the product's design principles, information
architecture, and interaction conventions. It is intended for contributors who
want to understand *why* the interface is shaped the way it is before changing
it. It is a rationale document, not a feature list.

## Who The Interface Is For

The primary user is a small operator doing repeat bookkeeping work for one or
more business entities. They are not exploring; they are processing a queue of
receipts and imports and deciding what to post to the books. Every design
decision optimizes for that repeated, deterministic work rather than for casual
browsing or first-time discovery.

## Core Invariant: Nothing Posts Automatically

The product's central promise is that automated processing (OCR, categorization,
and suggestion) *proposes* accounting entries, but a human always confirms
before anything is posted to the ledger. Suggestions become **drafts**; a person
reviews the draft and explicitly posts it.

The interface must continuously reinforce this invariant. Draft state should be
visible, posting should be a deliberate act, and no screen should imply that
work has been committed to the books when it has only been suggested. This is
both a trust property and a correctness property of a bookkeeping tool.

## Design Principles

1. **The next safe action should always be obvious.** On any screen, the single
   most useful next step is promoted above everything else.
2. **Context precedes action.** An active entity is a prerequisite for most
   work; actions that require entity context are not promoted until that context
   is established.
3. **Frequency drives prominence.** High-frequency decisions (review, confirm,
   post) are faster to reach than low-frequency configuration (settings, system
   administration).
4. **Diagnostics are available, not dominant.** OCR history, raw payloads, and
   suggestion internals are reachable when needed but never compete with the
   primary decision by default.
5. **Empty states are part of the product.** A screen with no data explains what
   it is for, why it is empty, and what to do next.

## Information Architecture

Navigation is organized by workflow zone rather than as a flat list of
destinations. Grouping by intent reduces scan time and makes the product easier
to learn, because related tasks live together and unrelated tasks do not compete
for equal attention.

| Zone | Contains | Purpose |
| --- | --- | --- |
| **Capture** | Receipts, Imports, Mileage | Get source documents into the system |
| **Review** | Review queue | Decide and confirm drafts |
| **Books** | Transactions, Accounts, Reports, Exports | The ledger and its outputs |
| **Settings** | User, Entity, System | Configuration and administration |

Settings and system administration sit in a secondary layer. Infrequent
configuration should never share visual weight with the daily bookkeeping loop.

### Entity Context Is First-Class

Almost all work is scoped to an active business entity. The active entity is
shown prominently in the header, and entity-dependent actions are demoted or
disabled until an entity is selected. When context is missing, the interface
gives explicit guidance ("Select an entity to upload receipts") rather than
presenting an empty-but-available state that leads to dead ends. Strong context
framing prevents invalid actions and makes the product feel deterministic.

## Screen Roles

### Home / Global Dashboard

Orients the user rather than splitting attention. When no entity is selected,
the primary action is to select or create one. Once an entity is active,
emphasis moves to the entity dashboard and the review queue. Summary content is
actionable (receipts ready, imports blocked, drafts pending) rather than
decorative recent-activity.

### Entity Dashboard

The operational landing page once an entity is active — an inbox for a single
business. It presents work-summary cards (ready for review, needs attention,
drafts pending post, unresolved processing errors), a primary action row
(upload receipt, start import, open review queue), and recent operational items.
This is the control center for daily work.

### Receipts

Structured as an operational queue with an intake shortcut, not a form with a
list attached. The list of existing receipts is the primary region for returning
users; the upload action stays available but advanced metadata fields collapse
behind an "Add details" affordance. List rows carry meaningful metadata (status,
amount, date, whether a draft exists), and empty states distinguish "no receipts
yet" from "no receipts match this filter."

### Imports

Follows the same hierarchy as Receipts. The most common intake mode (file
upload) is the default; the alternative (pasted CSV) is available but demoted,
with technical fields such as filename and content type collapsed under an
advanced section. Rows summarize processing state and surface blocking errors.

### Review Queue

The operational heart of the product, optimized for decision speed rather than
maximum simultaneous detail. Each item promotes exactly one primary action based
on its state:

- **Ready** for review -> open review
- **Blocked** / needs attention -> fix or retry
- **Processing** -> no action; show the current stage

Rerun and secondary controls remain reachable but move into a secondary action
row. Items group into consistent states (Processing, Ready, Blocked), and the
interface clearly separates item status, latest processing stage, and any
blocking error.

### Receipt Detail

Ordered by decision priority into three vertical bands:

1. **Review summary** — file preview, known amount/date/vendor, current status,
   suggestion confidence, and any blocking errors.
2. **Draft confirmation** — the primary editable section, where the user accepts
   or adjusts the proposed entry before posting.
3. **Technical detail** — OCR history, suggestion history, and raw payloads,
   collapsed by default.

The user's job here is to decide whether the draft is acceptable and then post.
Everything on the page either supports that decision or gets out of its way.

### Reports, Exports, Accounts, Transactions

These read as bookkeeping tools rather than generic admin tables. A page-level
summary precedes the detail list, sensible default date ranges replace blank
query forms, and exports are framed as outputs of a selected report scope rather
than detached tools. In accounting interfaces, pairing summary with detail lets
users build confidence before drilling into line items.

## Product Conventions

Beyond individual screens, several conventions apply across the app.

- **Draft lifecycle visibility.** Draft state is surfaced wherever it is
  relevant — a badge in receipt lists, a "drafts pending post" card on the
  entity dashboard, and a filter for draft-ready items. Because drafts embody
  the "suggest, then confirm" model, hiding them would obscure the product's
  core value.
- **Global feedback for background work.** Processing is asynchronous, so
  feedback cannot be confined to the page where an action started. A lightweight
  global notification pattern confirms that queued reruns and long-running jobs
  happened, even after the user has navigated away.
- **Search and sticky filters.** Operational pages remain usable as data grows
  by supporting search (for example by merchant or file name) and persistent
  filter scopes such as "needs review," "posted this month," or "has errors."
  Retrieval should not depend on scanning alone.
- **User-centered error messages.** Stable machine-readable error codes are
  useful to developers, but users see translated messages that explain the
  consequence and the next step ("Receipt already attached," "This account
  belongs to another entity," "Setup is still required").
- **Onboarding as just-in-time instruction.** Bookkeeping has domain-specific
  steps, so first-use guidance explains what setup creates, how the review queue
  works, and that posting is always manual — delivered in context rather than as
  a wall of documentation.

## Accessibility and Keyboard Workflows

Because the same operator uses the product repeatedly, accessibility is a
throughput concern, not only a compliance one. The interface maintains visible
focus states everywhere, gives filters and toggles a clear selected state,
supports keyboard traversal through review and draft-editing actions, and avoids
relying on icon-only affordances without labels.

## How Success Is Judged

UX quality is evaluated through observable operational outcomes rather than
subjective polish:

- fewer abandoned uploads and imports
- faster time from capture to posted transaction
- fewer invalid actions caused by missing entity context
- fewer repeat retries caused by unclear processing state
- fewer "what do I do next?" moments

These outcomes follow directly from the principles above: obvious next actions,
context before action, and diagnostics that inform without overwhelming.
