package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/openb00ks/openb00ks/internal/models"
)

const vendorRuleColumns = "vr.id, vr.entity_id, vr.match_type, vr.pattern, vr.account_id, vr.created_at"

type VendorRuleStore struct {
	db *DB
}

func NewVendorRuleStore(db *DB) *VendorRuleStore {
	return &VendorRuleStore{db: db}
}

func (s *VendorRuleStore) Create(ctx context.Context, rule models.VendorRule) (models.VendorRule, error) {
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO vendor_rules (entity_id, match_type, pattern, account_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, rule.EntityID, rule.MatchType, rule.Pattern, rule.AccountID)
	if err != nil {
		return models.VendorRule{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *VendorRuleStore) Update(ctx context.Context, id string, rule models.VendorRule) (models.VendorRule, error) {
	var updatedID string
	err := s.db.GetContext(ctx, &updatedID, `
		UPDATE vendor_rules
		SET match_type = $2, pattern = $3, account_id = $4
		WHERE id = $1
		RETURNING id
	`, id, rule.MatchType, rule.Pattern, rule.AccountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.VendorRule{}, ErrNotFound
		}
		return models.VendorRule{}, err
	}
	return s.GetByID(ctx, updatedID)
}

func (s *VendorRuleStore) GetByID(ctx context.Context, id string) (models.VendorRule, error) {
	row := VendorRuleRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+vendorRuleColumns+`
		FROM vendor_rules vr
		WHERE vr.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.VendorRule{}, ErrNotFound
		}
		return models.VendorRule{}, err
	}
	return vendorRuleFromRow(row), nil
}

func (s *VendorRuleStore) GetEntityID(ctx context.Context, id string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `
		SELECT vr.entity_id
		FROM vendor_rules vr
		WHERE vr.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return entityID, nil
}

func (s *VendorRuleStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM vendor_rules
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *VendorRuleStore) List(ctx context.Context, entityID string, limit int) ([]models.VendorRule, error) {
	if limit <= 0 {
		limit = 200
	}
	rows := []VendorRuleRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+vendorRuleColumns+`
		FROM vendor_rules vr
		WHERE vr.entity_id = $1
		ORDER BY vr.created_at DESC
		LIMIT $2
	`, entityID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.VendorRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, vendorRuleFromRow(row))
	}
	return out, nil
}

func (s *VendorRuleStore) FindMatching(ctx context.Context, entityID, vendor string) ([]models.VendorRule, error) {
	vendorNorm := normalizeVendor(vendor)
	rows := []VendorRuleRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+vendorRuleColumns+`
		FROM vendor_rules vr
		WHERE vr.entity_id = $1
		ORDER BY vr.created_at DESC
	`, entityID)
	if err != nil {
		return nil, err
	}
	matches := make([]models.VendorRule, 0)
	for _, row := range rows {
		rule := vendorRuleFromRow(row)
		if ruleMatches(rule, vendorNorm) {
			matches = append(matches, rule)
		}
	}
	return matches, nil
}

func vendorRuleFromRow(row VendorRuleRow) models.VendorRule {
	return models.VendorRule{
		ID:        row.ID,
		EntityID:  row.EntityID,
		MatchType: row.MatchType,
		Pattern:   row.Pattern,
		AccountID: row.AccountID,
		CreatedAt: row.CreatedAt,
	}
}

func normalizeVendor(vendor string) string {
	return strings.ToLower(strings.TrimSpace(vendor))
}

func ruleMatches(rule models.VendorRule, vendorNorm string) bool {
	pattern := normalizeVendor(rule.Pattern)
	switch rule.MatchType {
	case "exact":
		return vendorNorm == pattern
	case "contains":
		return strings.Contains(vendorNorm, pattern)
	default:
		return false
	}
}
