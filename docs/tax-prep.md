# Tax prep workflow

Goal: produce reviewable books and working papers for a tax year by entity.

Open B00KS produces bookkeeping workpapers and preparer handoff files. It does not produce or submit a tax return.

## Recommended flow

1. Create/select the entity.
2. Set the entity fiscal year if it is not the default calendar year.
3. Set up the chart of accounts and add account role tags for utilities, phone, and internet accounts where applicable.
4. Configure entity home-use allocation for the tax year when the business uses home office, cell phone, or home internet expenses.
5. Add vendor rules for common merchants and income sources.
6. Import bank/card CSVs from each source account.
7. Create statement records for imported bank/card periods with starting and ending balances.
8. Review import rows and exceptions.
9. Assign an account to each import row.
10. Post mapped rows into balanced journal entries.
11. Reconcile statements and resolve differences.
12. Add receipt and mileage records that support the year.
13. Open Tax prep and Preparer summary to clear blockers.
14. Export the tax pack for the entity and year.

## CSV import expectations

The importer accepts common bank/card CSV shapes:

- `Date, Description, Amount`
- `Date, Merchant, Amount`
- `Transaction Date, Payee, Debit, Credit`
- Headerless rows in `date, vendor, amount` order

Amounts are normalized to cents. Signed negative amounts are treated as outflows. Positive signed amounts and `Credit` columns are treated as inflows. `Debit` columns are treated as outflows.

Each parsed row carries:

- normalized date
- vendor/description
- memo
- amount in cents
- direction: `outflow`, `inflow`, or `unknown`
- duplicate fingerprint
- row-level parse errors when parsing fails

Processed imports also persist row records. Use `GET /imports/{id}/rows` to review them, `PATCH /imports/{id}/rows/{row_index}` to assign an account, and `POST /imports/{id}/rows/{row_index}/post` to create a balanced transaction for one row.

## Tax pack

Endpoint:

```text
GET /exports/tax-pack.zip?entity_id=<id>&year=2025
```

Readiness check:

```text
GET /reports/tax-readiness?entity_id=<id>&year=2025
```

Use this before generating the ZIP to find unposted rows, unmapped rows, duplicate-suspect rows, and parse errors.
Readiness checks are scoped to the requested tax year or date range, so unrelated imports from other periods do not block the current filing work.

The readiness response also includes grouped `actions` with links back to the import or receipt that needs attention. Use these as the first-pass punch list before reviewing the full exception detail.

The readiness response and `import-summary.csv` include reconciliation fields:

- imported outflow/inflow totals
- posted outflow/inflow totals
- mapped row count
- posted row count
- unposted row count
- duplicate row indexes

Readiness can also report tax-prep gaps beyond imports:

- receipts that still need review
- missing or partial entity home-use allocation
- missing account role tags for utility, cell phone, or internet allocation coverage
- statement balance differences, unposted statement rows, and unreconciled statements
- mileage months missing a reimbursement rate

Date range alternative:

```text
GET /exports/tax-pack.zip?entity_id=<id>&start_date=2025-01-01&end_date=2025-12-31
```

Files included:

- `profit-loss.csv`
- `general-ledger.csv`
- `transactions.csv`
- `mileage.csv`
- `import-summary.csv`
- `review-actions.csv`
- `blocking-summary.csv`
- `home-use-allocation.csv`
- `account-role-tags.csv`
- `statement-reconciliation.csv`
- `prep-checklist.csv`
- `prepared-package.csv`
- `exceptions.csv`
- `README.md`

The ledger and transaction CSVs include source provenance columns when available:

- `source_kind`
- `source_id`
- `source_name`
- `source_row`
- `source_vendor`
- `source_hash`

## Filing readiness checks

Before using the export for taxes:

- `exceptions.csv` should be empty or manually explained.
- Import totals should reconcile to bank/card statements.
- Statement starting and ending balances should reconcile in `statement-reconciliation.csv`.
- Use signed book balances: bank/asset balances are usually positive, while credit-card/loan liability balances are usually negative.
- `prep-checklist.csv` should show completed import, receipt, mileage, and allocation checks, or the exceptions should be explained.
- `home-use-allocation.csv` should match the business-use ratios you intend to give your preparer.
- `account-role-tags.csv` should identify the accounts used for utilities, phone, and internet allocation coverage.
- Expense categories should be reviewed for tax treatment.
- Income, owner draws/contributions, loans, assets, depreciation, payroll, 1099s, and sales tax should be checked outside this basic pack when applicable.

This export is a bookkeeping workpaper bundle, not a tax return.
