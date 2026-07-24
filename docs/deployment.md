# Deployment and local runtime design

## Goals

- Keep the app easy to run for OSS contributors.
- Keep the production story clean for Kubernetes.
- Avoid making local development depend on a single orchestration tool.
- Treat Postgres as the source of truth and search as derived infrastructure.

## Runtime components

- `api`: Go REST API, auth, domain logic, migrations, and health checks.
- `worker`: Go background processor for receipt/import jobs, OCR, rules, search retrieval, and AI suggestions.
- `web`: SvelteKit static build served by nginx in the current image.
- `postgres`: primary relational store.
- `receipt storage`: local filesystem in development; S3-compatible object storage in deployed environments.
- `typesense`: optional search and historical retrieval service.
- `ai provider`: optional BYOK provider, currently OpenAI through `github.com/spectrum-labs-tech/ai`.

## Local development modes

### Taskfile-first mode

This remains the simplest path for active development:

- `task dev/db` starts local Postgres.
- `task dev/api` runs the API.
- `task dev/worker` runs the worker.
- `task dev` runs the web app.

This mode is useful for contributors who want direct process output and fast iteration.

### Docker Compose mode

Compose is included for OSS onboarding, even if it is not the preferred daily workflow. It provides a one-command local stack for people who expect that shape.

Included services:

- `postgres`
- `typesense`
- `api`
- `worker`
- `web`

Optional later services:

- `minio` for S3-compatible receipt storage
- OpenTelemetry collector
- mail catcher, if email flows are added

Compose should use the same environment variables as Taskfile development. The goal is not a separate development mode; it is an alternate launcher for the same runtime.

## Kubernetes packaging

This repo may include reusable deployment packaging, but it should not include environment-specific deployment configuration.

The boundary is:

- allowed: generic Helm chart, Dockerfiles, Compose for local OSS onboarding
- not allowed: real cluster names, hostnames, production values, secrets, cert-manager assumptions, personal overlays, or GitOps environment state

Environment values should live outside this repo, usually in an infra or GitOps repository.

Chart path:

```text
charts/open-b00ks
```

Included Kubernetes resources:

- Deployment: `api`
- Deployment: `worker`
- Deployment: `web`
- Service: `api`
- Service: `web`
- Ingress: `web` and API routing
- Job: database migrations
- Secret: database URL, JWT secret, AI key, Typesense key, or reference to an existing secret
- ConfigMap: non-secret app config
- Optional transaction reindex Job

Postgres and Typesense default to external dependencies in the Helm chart. The chart does not bundle databases or search services. Production deployments should point at managed or separately operated services.

## Configuration principles

- Every runtime setting should be environment-driven.
- Local `.env` files, Compose env blocks, and Helm values should map to the same variable names.
- Secrets should never be baked into images.
- The API and worker should share the same database, queue, AI, storage, and search config.
- The web image should receive `PUBLIC_API_BASE_URL` at build time until runtime web config is introduced.

## Search and AI deployment

Typesense is optional, but when enabled it supports two flows:

- historical retrieval for suggestions
- fast user-facing transaction/receipt search

The worker should query Typesense before calling AI. The AI prompt should receive only compact, permission-scoped candidates. Postgres remains authoritative; Typesense can always be rebuilt.

AI remains optional. If `AI_PROVIDER=none` or provider config is invalid, the app should continue with rules and search-derived suggestions.

Transaction reindexing is available through multiple operational paths:

- CLI/Taskfile for local and deployment operations.
- Entity-scoped API reindexing for authenticated entity admins.

The API path must stay entity-scoped. Tenant-wide or all-entity reindexing belongs in ops-controlled CLI or Kubernetes jobs.

Every Typesense transaction query must include both tenant and entity scope. Missing scope is treated as an error so one entity's transactions cannot leak into another entity's results.

## Migration strategy

Local development can run migrations through Taskfile.

Kubernetes should run migrations as a Helm-managed Job before rolling out the API and worker. The API may still run bootstrap migrations for fresh self-hosted installs, but the deployed path should make migration execution explicit and visible.

## Implementation plan

1. Add Docker Compose for local OSS onboarding with Postgres and optional Typesense. Done.
2. Add search configuration and a no-op search provider so the app can run without Typesense. Done.
3. Add a Typesense indexing/search package and tests around candidate ranking. Done.
4. Feed historical candidates into the worker suggestion pipeline before AI. Done.
5. Add a transaction/receipt/import search API backed by the same search package. Done for the unified document index.
6. Add transaction search reindex paths and scope guards. Done.
7. Add `charts/open-b00ks` with API, worker, web, migration job, secrets, config, optional ingress, and reindex job. Done.
8. Add deployment docs showing Taskfile, Compose, and Helm as supported paths.

## Open decisions

- Whether Compose should include MinIO immediately or keep receipt storage local for v0.
- Whether Typesense should be required in Compose or enabled through a profile.
- Whether the web should move from build-time `PUBLIC_API_BASE_URL` to runtime config before Helm is added.
- Whether migrations should remain API bootstrap behavior in production or only run as an explicit job.
