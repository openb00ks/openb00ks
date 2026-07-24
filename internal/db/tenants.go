package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const tenantColumns = "t.id, t.name, t.created_at"
const tenantMembershipColumns = "tm.id, tm.tenant_id, t.name AS tenant_name, tm.user_id, tm.role, tm.created_at"

type TenantStore struct {
	db *DB
}

func NewTenantStore(db *DB) *TenantStore {
	return &TenantStore{db: db}
}

func (s *TenantStore) Create(ctx context.Context, name string) (models.Tenant, error) {
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id
	`, name)
	if err != nil {
		return models.Tenant{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *TenantStore) GetByID(ctx context.Context, id string) (models.Tenant, error) {
	row := TenantRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+tenantColumns+`
		FROM tenants t
		WHERE t.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Tenant{}, ErrNotFound
		}
		return models.Tenant{}, err
	}
	return tenantFromRow(row), nil
}

func (s *TenantStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM tenants`); err != nil {
		return 0, err
	}
	return count, nil
}

type TenantMembershipStore struct {
	db *DB
}

func NewTenantMembershipStore(db *DB) *TenantMembershipStore {
	return &TenantMembershipStore{db: db}
}

func (s *TenantMembershipStore) ListForUser(ctx context.Context, userID string, limit int) ([]models.TenantMembership, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []TenantMembershipRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+tenantMembershipColumns+`
		FROM tenant_memberships tm
		JOIN tenants t ON t.id = tm.tenant_id
		WHERE tm.user_id = $1
		ORDER BY tm.created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	memberships := make([]models.TenantMembership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, tenantMembershipFromRow(row))
	}
	return memberships, nil
}

func (s *TenantMembershipStore) Create(ctx context.Context, tenantID, userID string, role models.Role) (models.TenantMembership, error) {
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO tenant_memberships (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, userID, string(role))
	if err != nil {
		return models.TenantMembership{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *TenantMembershipStore) GetByID(ctx context.Context, id string) (models.TenantMembership, error) {
	row := TenantMembershipRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+tenantMembershipColumns+`
		FROM tenant_memberships tm
		JOIN tenants t ON t.id = tm.tenant_id
		WHERE tm.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TenantMembership{}, ErrNotFound
		}
		return models.TenantMembership{}, err
	}
	return tenantMembershipFromRow(row), nil
}

func (s *TenantMembershipStore) GetRole(ctx context.Context, tenantID, userID string) (models.Role, error) {
	var role string
	err := s.db.GetContext(ctx, &role, `
		SELECT tm.role
		FROM tenant_memberships tm
		WHERE tm.tenant_id = $1 AND tm.user_id = $2
	`, tenantID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return models.Role(role), nil
}

func tenantFromRow(row TenantRow) models.Tenant {
	return models.Tenant{
		ID:        row.ID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
	}
}

func tenantMembershipFromRow(row TenantMembershipRow) models.TenantMembership {
	return models.TenantMembership{
		ID:         row.ID,
		TenantID:   row.TenantID,
		TenantName: row.TenantName,
		UserID:     row.UserID,
		Role:       models.Role(row.Role),
		CreatedAt:  row.CreatedAt,
	}
}
