# Open B00KS Helm chart

This chart is generic runtime packaging for Open B00KS. It is not an environment deployment.

Keep real cluster values outside this repo, for example in an infra or GitOps repository.

## What it includes

- API Deployment and Service
- Worker Deployment
- Web Deployment and Service
- Optional Ingress
- Optional migration Job
- Optional transaction reindex Job
- Optional local receipt PVC

Postgres and Typesense are external dependencies by default. Configure them through `DATABASE_URL`, `TYPESENSE_URL`, and related secret values.

## Minimal values

```yaml
secrets:
  databaseURL: postgres://user:pass@postgres.example:5432/openb00ks?sslmode=require
  jwtSecret: change-me

config:
  corsAllowedOrigins: https://books.example.com
```

## Existing secret

```yaml
existingSecret: open-b00ks-secrets
secrets:
  create: false
```

The secret must contain:

- `DATABASE_URL`
- `JWT_SECRET`
- optional `OPENAI_API_KEY`
- optional `TYPESENSE_API_KEY`

## Reindexing

The chart can render a one-off transaction reindex Job:

```yaml
reindex:
  enabled: true
  tenantID: tenant-id
  entityID: entity-id
```

Leave `reindex.enabled=false` for normal installs and upgrades.
