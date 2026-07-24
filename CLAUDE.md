# Claude Code instructions

@AGENTS.md

The import above is the shared, tool-agnostic project context (the source of
truth lives in `AGENTS.md`). Claude Code reads only `CLAUDE.md`, so `AGENTS.md`
is re-exported here rather than duplicated.

Claude-specific notes:

- Keep edits aligned with the documented v0 scope in `/docs`.
- Preserve the bookkeeping invariants: balanced entries, immutable receipts,
  read-only suggestions.
- Do not add AI behavior that posts transactions or creates data.
- Prefer Taskfile commands for builds/tests; run integration tests via their
  Task target (e.g. `task go:integration`) so `.env.test` loads consistently.
