DROP INDEX IF EXISTS idx_entities_tenant_id;
DROP INDEX IF EXISTS idx_tenant_memberships_user_id;

DROP TABLE IF EXISTS tenant_memberships;

ALTER TABLE IF EXISTS refresh_tokens
  DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS entities
  DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS users
  DROP COLUMN IF EXISTS default_tenant_id;

DROP TABLE IF EXISTS tenants;
