# First-boot Wizard (Draft)

Goal: provide a safe, one-time setup flow to create the initial tenant + admin user.

## Why

- Fresh installs need a simple, guided path to create the first admin.
- We must prevent creating multiple “first admins” after initialization.
- Works for self-hosted and hosted environments.

## Core Requirements

- Only available when system setup is not marked complete.
- Create: tenant, admin user, default entity (optional), and membership.
- After setup, wizard is permanently disabled.
- Clear feedback and failure handling.

## Proposed API

### GET /setup/status

Returns whether setup is still required.

Response:

```
{ "required": true }
```

### POST /setup

Creates initial tenant + admin user. Disabled once initialized.

Request:

```
{
  "tenant_name": "Acme Holdings",
  "admin_email": "owner@example.com",
  "admin_password": "...",
  "default_entity_name": "Acme LLC" // optional
}
```

Response:

```
{
  "tenant_id": "...",
  "admin_user_id": "...",
  "entity_id": "..." // if created
}
```

## Backend Logic

- Setup is **required** when `system_settings.setup_complete` is false.
- POST /setup should:
  - Create tenant (name required).
  - Create admin user (email + password).
  - Create tenant membership (admin).
  - Set default tenant for the new user.
  - Optionally create default entity.
  - Mark `system_settings.setup_complete = true` and set `setup_completed_at`.
- If setup is not required, return `409 SETUP_ALREADY_COMPLETE`.

## UI Flow

- A `/setup` route is accessible only when `setup.required === true`.
- The login page performs a **single** `/setup/status` check on mount.
- If login is attempted before setup, API returns `SETUP_REQUIRED` and UI redirects to `/setup`.
- Form fields: tenant name, admin email, admin password, optional default entity name.
- After success: navigate to `/login` with success message.

## Security Considerations

- Endpoint should be disabled once initialized.
- Rate-limit POST /setup to reduce abuse.
- Strong password validation (length + complexity in UI, enforce minimum in API).
- Ensure logs do not print plaintext passwords.
- Avoid global setup checks in authenticated flows (only check on unauthenticated entry points).

## Open Questions

- Should we allow setup only on localhost in dev?
- Do we allow creating a default entity automatically, or require explicit?
- Should setup also configure AI provider + storage in future?
