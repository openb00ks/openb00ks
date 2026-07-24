# Contributing

Thanks for your interest in contributing to Open B00KS.

## Contributor License Agreement

Open B00KS is open source under **AGPL-3.0** and stewarded by Spectrum Labs LLC.
Before your first contribution can be merged, you must sign our
[Contributor License Agreement](./CLA.md).

You only sign once. When you open your first pull request, an automated check
posts a comment with a link to the CLA and instructions. Signing takes one
comment and is recorded against your GitHub account for all future
contributions.

Signing the CLA lets Spectrum Labs distribute Open B00KS under AGPL-3.0 and offer
it under separate commercial terms, which is how the project stays sustainable.
You keep full copyright to your own contributions — see [CLA.md](./CLA.md) for
the exact terms.

## Getting started

See [`AGENTS.md`](./AGENTS.md) for project context, invariants, and the full set
of dev commands. In short:

- Build, lint, test, and format are driven through the `Taskfile.yaml` targets.
- Run `task --list` to see available commands.
- Unit tests must pass without external resources; integration tests run through
  their Task target so `.env.test` is loaded consistently.

## Pull requests

1. Keep changes focused and aligned with the documented scope.
2. Preserve the bookkeeping invariants (balanced entries, immutable receipts).
3. Include tests that cover the happy path and at least one edge case.
4. Make sure the CLA check and CI are green before requesting review.
