# Chart templates

Account-set templates seed a starter chart of accounts when an entity is created, so a new entity is usable
by the receipt pipeline immediately — the classifier needs real expense accounts to choose from, and every
entity needs a `Cash` account for the double-entry credit leg.

## Approach

- Templates live in code as embedded JSON (`internal/templates/templates/*.json`, via `go:embed`).
- Each template is `{ key, name, accounts: [{ name, type, code }] }`, mapping directly to `models.Account`.
- Applied at entity creation — no per-entity DB schema. Seeding is idempotent (dedup by name) and always
  ensures a `Cash` account (`ensureCashAccount`), which the pipeline's credit leg looks up by name.
- Every template account carries a numbered chart-of-accounts `code` (assets `1xxx`, liabilities `2xxx`,
  equity `3xxx`, income `4xxx`, expenses `5xxx`); accounts list ordered by code. Codes are optional on
  hand-created accounts (`code` is nullable) and editable per account on the accounts screen.

## Available templates

- **`basic`** — the DEFAULT chart when no template is chosen: Cash, Accounts Receivable, Accounts Payable,
  Owner's Equity, Income, Expense.
- **`software-startup`** — subscription/services revenue + software & SaaS, cloud hosting, contractors,
  payroll, etc.
- **`property-management`** — rental/management-fee income, trust/escrow + security-deposits-held, repairs,
  property taxes, HOA, mortgage interest, etc.
- **`short-term-rental`** — rental + cleaning-fee income, turnover/consumables, platform fees, occupancy
  taxes, channel-manager software, etc.
- **`small-retailer`** — product + consignment sales, inventory, sales-tax-payable, COGS, booth/show fees,
  payment processing, marketplace fees, etc.

## How they're chosen

- **`GET /entity-templates`** (public) lists them (`key`, `name`, `account_count`) for the picker.
- **`POST /entities`** and first-run **`POST /setup`** accept a `template` key; empty/omitted → `basic`.
- The picker is exposed on both the entities page and the first-run setup page.

## Template shape

- `key` — stable id used by the API
- `name` — display label
- `accounts[]` — `{ name, type, code }`, where `type` ∈ `asset | liability | equity | income | expense`
  and `code` is a numbered chart-of-accounts code (assets `1xxx` … expenses `5xxx`)

## Future expansion

- **Role tags on template accounts**, so tax-prep allocation works out of the box (see `tax-prep.md`).
- DB-backed templates if per-tenant admin editing is ever needed.
