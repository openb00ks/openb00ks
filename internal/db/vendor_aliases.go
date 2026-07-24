package db

import "context"

// VendorAlias is one raw receipt vendor string that resolved to a vendor (the memoization ledger).
type VendorAlias struct {
	VendorID    string
	EntityID    string
	RawString   string
	Normalized  string
	Occurrences int
}

// VendorAliasStore backs vendor_aliases.
type VendorAliasStore struct {
	db *DB
}

func NewVendorAliasStore(db *DB) *VendorAliasStore { return &VendorAliasStore{db: db} }

// Record memoizes that rawString (with caller-computed normalized form) resolved to vendorID: it upserts
// the alias (occurrences++ on a repeat) and bumps the vendor's receipt_count + last_seen. The
// (entity_id, normalized) uniqueness is first-writer-wins on the vendor mapping — a normalized string
// already tied to another vendor is NOT remapped here (corrections happen via the vendors API). A blank
// normalized string is a no-op. Best-effort memoization: the two writes are not transactional.
func (s *VendorAliasStore) Record(ctx context.Context, vendorID, entityID, rawString, normalized string) error {
	if normalized == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vendor_aliases (vendor_id, entity_id, raw_string, normalized)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity_id, normalized) DO UPDATE SET
			occurrences = vendor_aliases.occurrences + 1,
			last_seen   = now()
	`, vendorID, entityID, rawString, normalized); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE vendors SET receipt_count = receipt_count + 1, last_seen = now(), updated_at = now()
		WHERE id = $1
	`, vendorID)
	return err
}

// RecordConfirmed is the human-authoritative alias write (the reviewer posting a receipt): unlike Record's
// first-writer-wins, on conflict it REASSIGNS the normalized string to vendorID. This is what lets a
// vendor correction take effect — the raw string moves from the AI's wrong guess to the reviewer's choice.
func (s *VendorAliasStore) RecordConfirmed(ctx context.Context, vendorID, entityID, rawString, normalized string) error {
	if normalized == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vendor_aliases (vendor_id, entity_id, raw_string, normalized)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity_id, normalized) DO UPDATE SET
			vendor_id   = EXCLUDED.vendor_id,
			occurrences = vendor_aliases.occurrences + 1,
			last_seen   = now()
	`, vendorID, entityID, rawString, normalized); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE vendors SET receipt_count = receipt_count + 1, last_seen = now(), updated_at = now()
		WHERE id = $1
	`, vendorID)
	return err
}

// ListNormalized returns a vendor's distinct normalized aliases, most-seen first — the set fed to the
// _vendors search document for retrieval.
func (s *VendorAliasStore) ListNormalized(ctx context.Context, vendorID string) ([]string, error) {
	var out []string
	if err := s.db.SelectContext(ctx, &out, `
		SELECT normalized FROM vendor_aliases
		WHERE vendor_id = $1
		ORDER BY occurrences DESC, last_seen DESC
	`, vendorID); err != nil {
		return nil, err
	}
	return out, nil
}
