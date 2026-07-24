-- The vendor the AI pipeline resolved for a receipt (matched or newly created), plus the raw receipt
-- vendor string. Persisted so the reviewer-feedback loop can, when a human posts the receipt's draft,
-- update that vendor's default account (overruling the AI's classification) and reinforce its alias from
-- the confirmed entry. Nullable: null for non-receipt kinds, receipts with no legible vendor, or receipts
-- processed before this column existed. ON DELETE SET NULL so removing a vendor doesn't orphan receipts.
ALTER TABLE receipts
    ADD COLUMN resolved_vendor_id  UUID REFERENCES vendors (id) ON DELETE SET NULL,
    ADD COLUMN resolved_vendor_raw TEXT;
