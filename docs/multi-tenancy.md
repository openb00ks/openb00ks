# Multi-Tenancy Plan (Draft)

This document outlines a baseline multi-tenant architecture for Open B00KS. It is intended for both self-hosted and hosted deployments. The default model uses a shared database with strong tenant isolation.

## Goals

- Enable SaaS with a shared database and strong tenant isolation.
- Preserve OSS self-hosting and allow single-tenant deployments.
- Keep the initial implementation simple and testable.
- Support auditability and future RLS hardening in Postgres.

## Terminology

- **Tenant**: The owning organization/account in SaaS (may map to “entity group”).
- **Entity**: The bookkeeping unit (existing concept).
- **User**: A human account; users can belong to one or more tenants and entities.

## Approach (Phase 1: App-Level Isolation)

- Introduce a top-level `tenants` table.
- Add `tenant_id` to root tables only (see schema section) and derive scope through `entities`.
- Ensure every read/write path scopes by `tenant_id` via entity joins.
- Enforce tenant membership in auth middleware and stores.
- Keep data exports and reporting tenant-scoped.
- Support cross-tenant accountants via explicit membership and tenant switching.

## Schema Changes (Draft)

- `tenants`:
  - `id` (uuid, pk)
  - `name` (text)
  - `created_at`
- `users` (if needed):
  - `default_tenant_id` (nullable)
- `tenant_memberships`:
  - `tenant_id`
  - `user_id`
  - `role` (admin/accountant/user)
  - `created_at`
- Add `tenant_id` to:
  - `entities`
  - `tenant_memberships`
  - `refresh_tokens` (optional, if we want refresh tokens scoped to tenant)
  - `users.default_tenant_id` (optional)

All other tables remain entity-scoped and infer tenant via `entities`.

## Auth + Session

- Authentication returns both `user_id` and `tenant_id` (active tenant).
- Client stores `activeTenantId` and `activeEntityId`.
- Users can switch tenants; switching issues a new token with the selected tenant claim.
- Server middleware validates:
  - user is member of tenant
  - entity belongs to tenant

## Accountant Cross-Tenant Flow (Example)

- An accountant is invited to multiple tenants (via `tenant_memberships`).
- The UI lists available tenants and allows switching the active tenant.
- Switching tenants reissues a token with the new `tenant_id` claim.
- Entity access is still enforced per-tenant via `entity_users`.

## API Changes (Draft)

- Add tenant context to requests:
  - Primary: `tenant_id` in auth token claims
  - Optional: `X-Tenant-ID` header for admin tooling (not required for normal clients)
- Ensure all endpoints filter by tenant (via entity joins):
  - Entities: `/entities` returns only tenant entities.
  - Receipts/Transactions/Suggest: derive tenant by entity ownership.

## Preferences

- Preferences can be tenant-scoped (default entity within a tenant).
- Option: keep user-global prefs (theme) separate from tenant prefs (default entity).

## Data Isolation Tests

- Add integration tests:
  - Cross-tenant data fetch returns 404/forbidden.
  - Cross-tenant mutations rejected.
  - Export endpoints only return tenant data.

## RLS Hardening (Phase 2)

- Add Postgres RLS policies on tenant-scoped tables.
- Use a per-request `SET app.tenant_id = ...` and RLS policies that join through `entities`, e.g.:
  - `USING (entity_id IN (SELECT id FROM entities WHERE tenant_id = current_setting('app.tenant_id')::uuid))`
- Keep app-level checks for defense-in-depth.

## Deployment Notes

- Multi-tenancy is core architecture for shared deployments.
- Hosted value typically comes from managed ops (backups, upgrades, monitoring, support).
- Self-hosting remains first-class and can be single-tenant or multi-tenant.

## Open Questions

- Confirm tenant owns multiple entities.
- Should `users` be global or tenant-scoped? (lean global with memberships)
- Should AI usage/costs roll up per tenant for billing?
- Should refresh tokens be tenant-scoped or user-scoped with tenant claims?

## Next Steps

- Confirm tenant/entity relationship.
- Decide on auth token tenant claim vs header (token claim default).
- Create migration plan (backfill tenant_id for existing data).
- Implement middleware and store filters.
- Add integration tests for isolation.
