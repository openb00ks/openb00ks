# Testing guidelines

## Integration tests

- Integration tests must be rerunnable N times without requiring a clean database.
- Use unique values for any fields with unique constraints (email, entity name, etc.).
- Do not rely on empty tables or static IDs.
- Avoid destructive cleanup that assumes ownership of shared data.
- Prefer time-based or random suffixes to avoid collisions.

## Unit tests

- Keep unit tests deterministic and fast.
- Avoid any dependency on external services or environment state.
