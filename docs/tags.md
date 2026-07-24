# Tags and metadata (v0 draft)

## Goals

- Fast filtering and lookup across receipts, mileage, and transactions.
- Use tags as context to improve suggestions.
- Keep tags lightweight and user-editable.

## Tag model

- Tags are free-form strings scoped to an entity.
- Tags may be attached to receipts, mileage entries, and transactions.
- Tags should be indexed for search.

## Suggestion usage

- Tags can be used to fetch a small set of posted transactions as reference context.
- Tag-derived context is advisory only; user review is required.

## Metadata

- Store optional metadata key/value pairs for receipts and mileage (e.g., project, client, location).
- Metadata is not used for posting without user confirmation.
