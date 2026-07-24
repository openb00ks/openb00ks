package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Vendor is a first-class vendor for an entity (populated by the receipt pipeline; usable by the UI).
// ReceiptCount + LastSeen are memoization signals (how many receipts resolved here, and when last).
type Vendor struct {
	ID               string
	EntityID         string
	Name             string
	NormalizedName   string
	MatchPattern     string
	TaxID            string
	Website          string
	DefaultAccountID string
	ReceiptCount     int
	LastSeen         time.Time
}

// VendorStore backs the vendors table.
type VendorStore struct {
	db *DB
}

func NewVendorStore(db *DB) *VendorStore { return &VendorStore{db: db} }

// Create upserts a vendor keyed by (entity_id, normalized_name). On conflict it fills in any missing
// match pattern / default account / tax id / website without overwriting existing values, and returns
// the row. NormalizedName must be set by the caller (pipeline.NormalizeVendorName).
func (s *VendorStore) Create(ctx context.Context, v Vendor) (Vendor, error) {
	var id string
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO vendors (entity_id, name, normalized_name, match_pattern, tax_id, website, default_account_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, '')::uuid)
		ON CONFLICT (entity_id, normalized_name) DO UPDATE SET
			match_pattern      = COALESCE(vendors.match_pattern, EXCLUDED.match_pattern),
			default_account_id = COALESCE(vendors.default_account_id, EXCLUDED.default_account_id),
			tax_id             = COALESCE(vendors.tax_id, EXCLUDED.tax_id),
			website            = COALESCE(vendors.website, EXCLUDED.website),
			updated_at         = now()
		RETURNING id::text
	`, v.EntityID, v.Name, v.NormalizedName, v.MatchPattern, v.TaxID, v.Website, v.DefaultAccountID).Scan(&id)
	if err != nil {
		return Vendor{}, err
	}
	v.ID = id
	return v, nil
}

type vendorRow struct {
	ID               string         `db:"id"`
	EntityID         string         `db:"entity_id"`
	Name             string         `db:"name"`
	NormalizedName   string         `db:"normalized_name"`
	MatchPattern     sql.NullString `db:"match_pattern"`
	TaxID            sql.NullString `db:"tax_id"`
	Website          sql.NullString `db:"website"`
	DefaultAccountID sql.NullString `db:"default_account_id"`
	ReceiptCount     int            `db:"receipt_count"`
	LastSeen         sql.NullTime   `db:"last_seen"`
}

const vendorSelect = `SELECT id::text AS id, entity_id::text AS entity_id, name, normalized_name,
	match_pattern, tax_id, website, default_account_id::text AS default_account_id,
	receipt_count, last_seen FROM vendors`

// ListForEntity returns an entity's vendors (the pool the pipeline ranks candidates from).
func (s *VendorStore) ListForEntity(ctx context.Context, entityID string, limit int) ([]Vendor, error) {
	if limit <= 0 {
		limit = 500
	}
	rows := []vendorRow{}
	if err := s.db.SelectContext(ctx, &rows, vendorSelect+` WHERE entity_id = $1 ORDER BY name LIMIT $2`, entityID, limit); err != nil {
		return nil, err
	}
	out := make([]Vendor, 0, len(rows))
	for _, r := range rows {
		out = append(out, vendorFromRow(r))
	}
	return out, nil
}

// SetDefaultAccount overwrites a vendor's default expense account (the reviewer-feedback path: a human
// posting a receipt overrules the AI's classification). Returns ErrNotFound if the vendor doesn't exist.
func (s *VendorStore) SetDefaultAccount(ctx context.Context, id, accountID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE vendors SET default_account_id = NULLIF($2, '')::uuid, updated_at = now()
		WHERE id = $1
	`, id, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByID returns a single vendor, or ErrNotFound.
func (s *VendorStore) GetByID(ctx context.Context, id string) (Vendor, error) {
	var r vendorRow
	err := s.db.GetContext(ctx, &r, vendorSelect+` WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Vendor{}, ErrNotFound
	}
	if err != nil {
		return Vendor{}, err
	}
	return vendorFromRow(r), nil
}

// GetEntityID returns the owning entity of a vendor (for authz resolution), or ErrNotFound.
func (s *VendorStore) GetEntityID(ctx context.Context, id string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `SELECT entity_id::text FROM vendors WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return entityID, nil
}

// Update overwrites a vendor's editable fields (unlike Create, which COALESCE-merges). NormalizedName
// must be set by the caller. Returns ErrNotFound if the id doesn't exist, or ErrConflict if the new
// normalized name collides with another vendor of the same entity.
func (s *VendorStore) Update(ctx context.Context, id string, v Vendor) (Vendor, error) {
	var r vendorRow
	err := s.db.GetContext(ctx, &r, `
		UPDATE vendors SET
			name = $2, normalized_name = $3, match_pattern = NULLIF($4, ''), tax_id = NULLIF($5, ''),
			website = NULLIF($6, ''), default_account_id = NULLIF($7, '')::uuid, updated_at = now()
		WHERE id = $1
		RETURNING id::text AS id, entity_id::text AS entity_id, name, normalized_name,
			match_pattern, tax_id, website, default_account_id::text AS default_account_id,
			receipt_count, last_seen`,
		id, v.Name, v.NormalizedName, v.MatchPattern, v.TaxID, v.Website, v.DefaultAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return Vendor{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Vendor{}, ErrConflict
	}
	if err != nil {
		return Vendor{}, err
	}
	return vendorFromRow(r), nil
}

// Delete removes a vendor, returning ErrNotFound if it didn't exist.
func (s *VendorStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM vendors WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByNormalized is the deterministic exact match (folded name).
func (s *VendorStore) FindByNormalized(ctx context.Context, entityID, normalized string) (Vendor, bool, error) {
	var r vendorRow
	err := s.db.GetContext(ctx, &r, vendorSelect+` WHERE entity_id = $1 AND normalized_name = $2`, entityID, normalized)
	if errors.Is(err, sql.ErrNoRows) {
		return Vendor{}, false, nil
	}
	if err != nil {
		return Vendor{}, false, err
	}
	return vendorFromRow(r), true, nil
}

func vendorFromRow(r vendorRow) Vendor {
	return Vendor{
		ID: r.ID, EntityID: r.EntityID, Name: r.Name, NormalizedName: r.NormalizedName,
		MatchPattern: r.MatchPattern.String, TaxID: r.TaxID.String, Website: r.Website.String,
		DefaultAccountID: r.DefaultAccountID.String,
		ReceiptCount:     r.ReceiptCount, LastSeen: r.LastSeen.Time,
	}
}
