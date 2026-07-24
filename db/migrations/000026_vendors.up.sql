-- First-class vendors, populated by the receipt pipeline (a matched or AI-recommended vendor) and
-- usable by the UI. The pipeline retrieves candidates from here to MATCH a receipt's vendor against
-- known ones (feeding the vendor-match stage a shortlist), and default_account_id memoizes the
-- expense account so a matched vendor pre-fills it. Distinct from vendor_rules (explicit user-defined
-- pattern→account rules); a vendor may back a rule but need not.
CREATE TABLE vendors (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id          UUID        NOT NULL REFERENCES entities (id) ON DELETE CASCADE,
    name               TEXT        NOT NULL, -- canonical display name
    normalized_name    TEXT        NOT NULL, -- folded name for exact match / dedup
    match_pattern      TEXT,                 -- distinctive substring for auto-matching receipts
    tax_id             TEXT,
    website            TEXT,
    default_account_id UUID        REFERENCES accounts (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, normalized_name)
);

CREATE INDEX vendors_entity_idx ON vendors (entity_id);
