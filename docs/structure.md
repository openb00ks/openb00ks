# Project structure (v0)

## Goals

- Keep Go backend at repo root (`go.mod` at top).
- Isolate web app under `/web`.
- Keep Dockerfiles and build tooling at the repo root (Taskfile-driven).
- Keep docs centralized under `/docs`.

## Proposed layout

```
/ (repo root)
  go.mod, go.sum
  Taskfile.yaml, taskfiles/       # task runner + split task files
  Dockerfile, Dockerfile.worker, Dockerfile.ops-scheduler
  compose.yaml                    # local full-stack dev
  /cmd
    /api                          # REST API server
    /worker                       # async job worker
    /ops-scheduler                # recurring tasks (backups, search reconcile)
    /receipt-bench                # AI-pipeline eval harness
  /internal                       # domain logic + adapters
    /auth /domain /http /models
    /db /storage /search          # Postgres, object storage, Typesense adapters
    /queue /ops /pipeline
    /aibatch /aiconfig /suggest /ocr /eval /vendormemo /receiptbatch
    /config /logging /telemetry /migrate /importer /templates /testutil
  /web                            # SvelteKit + Tailwind
    /src /static
  /charts/open-b00ks              # Helm chart
  /db/migrations                  # SQL migrations
  /docs                           # architecture + design docs
```

## Notes

- Go package boundaries are internal-only for domain logic and adapters.
- `internal/http` handles routing, middleware, and handlers.
- `internal/storage` holds Postgres + object storage adapters.
- `internal/suggest` contains rule-based suggestions and AI provider interface.
- `web` is a standard SvelteKit project with Tailwind.
- Capacitor setup lives under `web/` when introduced (community practice).
