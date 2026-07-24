CREATE TABLE account_role_tags (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  role_tag TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, role_tag),
  CHECK (role_tag IN ('utilities', 'cell_phone', 'internet'))
);

CREATE INDEX idx_account_role_tags_role_tag ON account_role_tags(role_tag);
