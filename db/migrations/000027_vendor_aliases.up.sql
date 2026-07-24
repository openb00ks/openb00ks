-- Vendor aliases: the memoization ledger. Every raw receipt vendor string that resolves to a vendor
-- (whether by exact/AI match or by creating a new vendor) is recorded here, normalized. The pipeline
-- retrieves candidates against this growing set, so matching accuracy improves as receipts accrue.
-- UNIQUE(entity_id, normalized) enforces one vendor per normalized
-- string within an entity.
CREATE TABLE vendor_aliases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id   UUID        NOT NULL REFERENCES vendors (id) ON DELETE CASCADE,
    entity_id   UUID        NOT NULL REFERENCES entities (id) ON DELETE CASCADE,
    raw_string  TEXT        NOT NULL, -- the receipt vendor string as seen
    normalized  TEXT        NOT NULL, -- NormalizeVendorName(raw_string)
    occurrences INTEGER     NOT NULL DEFAULT 1,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, normalized)
);

CREATE INDEX vendor_aliases_vendor_idx ON vendor_aliases (vendor_id);

-- Confidence + recency signals surfaced to search (receipt_count for tiebreak, last_seen for recency
-- so reference-data vendors have a natural default sort in the _vendors collection).
ALTER TABLE vendors ADD COLUMN receipt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vendors ADD COLUMN last_seen TIMESTAMPTZ;
