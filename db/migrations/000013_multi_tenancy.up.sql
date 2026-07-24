CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users
  ADD COLUMN default_tenant_id UUID REFERENCES tenants(id);

CREATE TABLE tenant_memberships (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, user_id)
);

ALTER TABLE entities
  ADD COLUMN tenant_id UUID REFERENCES tenants(id);

ALTER TABLE refresh_tokens
  ADD COLUMN tenant_id UUID REFERENCES tenants(id);

DO $$
DECLARE
  tid UUID;
BEGIN
  INSERT INTO tenants (name) VALUES ('Default Tenant') RETURNING id INTO tid;

  UPDATE entities SET tenant_id = tid WHERE tenant_id IS NULL;
  UPDATE users SET default_tenant_id = tid WHERE default_tenant_id IS NULL;
  UPDATE refresh_tokens SET tenant_id = tid WHERE tenant_id IS NULL;

  INSERT INTO tenant_memberships (tenant_id, user_id, role)
  SELECT tid, u.id, CASE WHEN u.is_admin THEN 'admin' ELSE 'user' END
  FROM users u
  WHERE NOT EXISTS (
    SELECT 1 FROM tenant_memberships tm WHERE tm.tenant_id = tid AND tm.user_id = u.id
  );
END $$;

ALTER TABLE entities
  ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE refresh_tokens
  ALTER COLUMN tenant_id SET NOT NULL;

CREATE INDEX idx_entities_tenant_id ON entities(tenant_id);
CREATE INDEX idx_tenant_memberships_user_id ON tenant_memberships(user_id);
