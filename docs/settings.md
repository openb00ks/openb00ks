# Settings Model (Draft)

This document outlines a long-term settings model for Open B00KS. The goal is to make settings extensible without forcing frequent schema churn. We can implement pieces incrementally.

## Goals

- Provide a stable place for future configuration without broad migrations.
- Separate scopes clearly (system, tenant, entity, user).
- Allow OSS self-hosting to keep things simple while still supporting SaaS later.
- Avoid coupling settings to feature flags or billing (those can be added later).

## Scopes

### System settings

- Scope: entire deployment (single-tenant OSS or multi-tenant SaaS).
- Owner: system admin / operator.
- Examples: max upload sizes, feature flags, maintenance mode, AI provider limits.

### Tenant settings

- Scope: one tenant (SaaS account).
- Owner: tenant admins.
- Examples: default chart template, export formats, retention policies, usage caps.

### Entity settings

- Scope: one entity (bookkeeping unit).
- Owner: entity admins.
- Examples: default accounts, reporting prefs, AI context hints, tags behavior.

### User settings

- Scope: one user.
- Owner: user.
- Examples: theme, default entity, UI preferences.

## Current State

- **User settings**: `user_preferences` table + `/me/preferences` (theme, default_entity_id).
- **Entity settings**: `entities.suggestion_context` exists, but no general settings table.
- **Tenant settings**: not implemented yet.
- **System settings**: `system_settings` is DB-backed for mutable flags via `/settings/system` (admin-only).
- **System integration config**: AI provider/model + receipt storage details are still environment-derived and returned read-only in `/settings/system` responses.

## Proposed Data Model (minimal, extensible)

### system_settings (current)

- `setup_complete` (bool)
- `setup_completed_at` (timestamptz)
- `settings_json` (jsonb)
- `updated_at` (timestamptz)

### tenant_settings

- `tenant_id` (uuid, pk, fk tenants)
- `settings_json` (jsonb)
- `updated_at` (timestamptz)

### entity_settings

- `entity_id` (uuid, pk, fk entities)
- `settings_json` (jsonb)
- `updated_at` (timestamptz)

### user_preferences (existing)

- `user_id` (uuid, pk, fk users)
- `default_entity_id` (uuid, fk entities)
- `theme` (text)
- `created_at`, `updated_at`

## Access & Ownership

- System settings: system admin only.
- Tenant settings: tenant admin only (and must be in token tenant).
- Entity settings: entity admin only (must be entity member in tenant).
- User settings: the user themselves.

## Storage Strategy

- Use a single `settings_json` blob per scope to avoid churn.
- Enforce shape at the application layer (schema validation optional later).
- Version settings per scope if/when we need migrations of JSON shape.

## API Notes

- `GET /settings/system` (admin)
- `PATCH /settings/system` (admin)

- `GET /settings/tenant` (tenant admin)
- `GET /settings/entity/:id` (entity admin)
- `GET /me/preferences` (user)

- `PATCH /settings/tenant`
- `PATCH /settings/entity/:id`
- `PATCH /me/preferences`

## Open Questions

- Should `entities.suggestion_context` move into `entity_settings` or remain a column?
- Do we want a `tenant_defaults` section that can flow into new entities?
- What encryption approach for tenant API keys? (AWS KMS, system-level symmetric key, etc.)
- Should we support provider-specific models per tenant, or just key override?

## Resolved Questions

- **AI keys at system vs tenant level?** → Both, with fallback. System provides default, tenant can BYOK.
- **Entity-level AI keys?** → No. Tenant is the billing/ownership boundary. Entity is just an accounting unit.
- **System settings in DB or env-only?** -> Hybrid for v0: mutable settings are DB-backed (`system_settings.settings_json`), while deployment-level integration config remains env-driven.
- **What if no AI key anywhere?** → Graceful degradation. Pipeline continues with rules-only suggestions.

## AI Configuration

AI suggestions are optional. The system must work without any AI provider configured.

### Deployment Modes

**OSS Single-Tenant** (simplest)

- Operator deploys for one business/user
- AI key set via environment variable or K8s secret
- No tenant-level config needed
- Example: freelancer self-hosting for personal use

**OSS Multi-Tenant** (self-hosted, multiple businesses)

- Operator hosts for multiple tenants (e.g., accounting firm with clients)
- System-level key optional (operator may not want to pay for all tenants)
- Each tenant can BYOK (bring your own key)
- Example: bookkeeper hosting for 5 small business clients

**SaaS** (hosted, shared infrastructure)

- Operator provides system-level key shared across tenants
- Tenants have usage limits (requests/month, tokens/month)
- Tenants can optionally BYOK to bypass limits or use preferred provider
- Example: hosted Open B00KS with freemium AI tier

### Resolution Order

```
Tenant AI config → System AI config → Graceful degradation (rules only)
```

When resolving AI configuration for a request:

1. Check tenant settings for `ai_provider` + `ai_api_key`
2. If not set, fall back to system config (env vars)
3. If neither available, continue without AI (rules-only mode)

### Multi-Provider Support

Supported providers (extensible via driver registration):

- `openai` - OpenAI API
- `anthropic` - Anthropic API (Claude) — *planned; v0 implements `openai` only*
- `none` - Explicitly disabled

Future candidates: Azure OpenAI, Google Vertex, local models (Ollama).

### System-Level Config (Environment Variables)

```bash
# Provider selection
AI_PROVIDER=openai              # or anthropic, none

# OpenAI
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-5-nano

# Anthropic
ANTHROPIC_API_KEY=sk-ant-...
ANTHROPIC_MODEL=claude-3-haiku-20240307

# Cost tracking (for usage reporting)
AI_INPUT_CENTS_PER_1K_TOKENS=15
AI_OUTPUT_CENTS_PER_1K_TOKENS=60
```

### Tenant-Level Config (in `tenant_settings.settings_json`)

```json
{
  "ai": {
    "provider": "openai",
    "api_key_encrypted": "...",
    "model": "gpt-5-nano",
    "enabled": true
  }
}
```

Notes:

- `api_key_encrypted`: Tenant keys stored encrypted at rest (system encryption key)
- `enabled`: Tenant admin can disable AI even if key is set
- If `provider` set but no key, uses system key with tenant's provider/model prefs

### Usage Limits (SaaS Mode)

When system provides the AI key, tenants may have usage limits:

```json
{
  "ai": {
    "enabled": true,
    "limits": {
      "requests_per_month": 1000,
      "tokens_per_month": 500000
    }
  }
}
```

Limit enforcement:

- Track usage in `tenant_ai_usage` table (tenant_id, month, requests, tokens)
- When limit exceeded: graceful degradation to rules-only
- UI shows: "AI limit reached. Upgrade or add your own API key."
- BYOK tenants bypass system limits (they pay their own provider)

### Graceful Degradation

The pipeline never fails due to missing/exhausted AI. Degradation levels:

| Scenario              | Behavior           | `suggestion.provider`  | `suggestion.status`         |
| --------------------- | ------------------ | ---------------------- | --------------------------- |
| AI available          | Full AI suggestion | `openai` / `anthropic` | `succeeded`                 |
| AI unavailable        | Rules engine only  | `rules`                | `succeeded`                 |
| No rules match        | Empty suggestion   | `none`                 | `skipped`                   |
| AI call fails         | Retry then degrade | `openai`               | `failed` → `rules` fallback |
| Tenant limit exceeded | Rules only         | `rules`                | `limit_exceeded`            |

In all cases:

- Receipt upload succeeds
- OCR runs (if configured)
- Draft created (may be empty)
- Receipt moves to `ready_for_review`
- User can manually enter accounts

### Config Resolution Logic (Pseudocode)

```go
type AIConfig struct {
    Available     bool
    Provider      string  // "openai", "anthropic", "none"
    APIKey        string
    Model         string
    LimitExceeded bool
    Source        string  // "tenant", "system", "none"
}

func ResolveAIConfig(ctx context.Context, tenantID string) AIConfig {
    tenant := getTenantSettings(tenantID)
    system := getSystemConfig()

    // Tenant BYOK takes priority
    if tenant.AI.Enabled && tenant.AI.APIKey != "" {
        return AIConfig{
            Available: true,
            Provider:  tenant.AI.Provider,
            APIKey:    decrypt(tenant.AI.APIKey),
            Model:     tenant.AI.Model,
            Source:    "tenant",
        }
    }

    // Tenant uses system key (check limits)
    if tenant.AI.Enabled && system.AIProvider != "none" && system.AIAPIKey != "" {
        if exceedsLimit(tenantID, tenant.AI.Limits) {
            return AIConfig{
                Available:     false,
                LimitExceeded: true,
                Source:        "system",
            }
        }
        return AIConfig{
            Available: true,
            Provider:  coalesce(tenant.AI.Provider, system.AIProvider),
            APIKey:    system.AIAPIKey,
            Model:     coalesce(tenant.AI.Model, system.AIModel),
            Source:    "system",
        }
    }

    // No AI available
    return AIConfig{Available: false, Source: "none"}
}
```

### UI Indicators

Settings page should show AI status:

| State                     | Display                                                         |
| ------------------------- | --------------------------------------------------------------- |
| Tenant BYOK active        | "AI: Enabled (your API key)"                                    |
| Using system key          | "AI: Enabled (system provided)"                                 |
| Using system key + limits | "AI: Enabled (450/1000 requests this month)"                    |
| Limit exceeded            | "AI: Limit reached. Add your own key or wait until next month." |
| Not configured            | "AI: Not configured. Suggestions use rules only."               |
| Explicitly disabled       | "AI: Disabled by admin"                                         |

### Security Considerations

- Tenant API keys encrypted at rest using system-level encryption key
- Keys never logged or returned in API responses
- Keys never sent to frontend; backend resolves and uses directly
- Audit log records AI usage but not key values
- Key rotation: tenant can update key anytime via settings API

### Schema Additions

```sql
-- Usage tracking for SaaS limits
CREATE TABLE tenant_ai_usage (
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    month           DATE NOT NULL,  -- first of month
    request_count   BIGINT NOT NULL DEFAULT 0,
    prompt_tokens   BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    cost_cents      BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, month)
);
```

## Implementation Phasing

Phase 1 (docs only) ← current

- Keep current behavior, add this document for guidance.

Phase 2 (schema only)

- Add `tenant_settings`, `entity_settings` tables with `settings_json`.
- Add `tenant_ai_usage` table for usage tracking.
- No endpoints required; store is optional.

Phase 3 (graceful degradation)

- Implement `ResolveAIConfig` with system-only support.
- Worker checks AI availability before calling provider.
- Record `suggestion.status` to indicate degradation reason.

Phase 4 (tenant BYOK)

- Add encrypted key storage in tenant settings.
- Add tenant settings API endpoints.
- Add settings UI for tenant admins.

Phase 5 (usage limits)

- Implement usage tracking in worker.
- Add limit checking to `ResolveAIConfig`.
- Add usage display in tenant settings UI.
