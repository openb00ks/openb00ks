package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const entityColumns = "e.id, e.tenant_id, e.name, e.suggestion_context, e.fiscal_year_start_month, e.fiscal_year_start_day, e.created_at"

type EntityStore struct {
	db *DB
}

func NewEntityStore(db *DB) *EntityStore {
	return &EntityStore{db: db}
}

func (s *EntityStore) ListForUser(ctx context.Context, tenantID, userID string, limit int) ([]models.Entity, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []EntityRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+entityColumns+`
		FROM entities e
		JOIN entity_users eu ON eu.entity_id = e.id
		WHERE eu.user_id = $1 AND e.tenant_id = $2
		ORDER BY e.created_at DESC
		LIMIT $3
	`, userID, tenantID, limit)
	if err != nil {
		return nil, err
	}
	entities := make([]models.Entity, 0, len(rows))
	for _, row := range rows {
		entities = append(entities, entityFromRow(row))
	}
	return entities, nil
}

func (s *EntityStore) CreateWithOwner(ctx context.Context, tenantID, userID, name, suggestionContext string) (models.Entity, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Entity{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var id string
	err = tx.GetContext(ctx, &id, `
		INSERT INTO entities (tenant_id, name, suggestion_context)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, name, suggestionContext)
	if err != nil {
		return models.Entity{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO entity_users (user_id, entity_id, role)
		VALUES ($1, $2, $3)
	`, userID, id, string(models.RoleAdmin))
	if err != nil {
		return models.Entity{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Entity{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *EntityStore) Update(ctx context.Context, tenantID, entityID string, name *string, suggestionContext *string, fiscalYearStartMonth, fiscalYearStartDay *int) (models.Entity, error) {
	if name == nil && suggestionContext == nil && fiscalYearStartMonth == nil && fiscalYearStartDay == nil {
		return models.Entity{}, errors.New("no fields to update")
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		UPDATE entities
		SET name = COALESCE($2, name),
		    suggestion_context = COALESCE($3, suggestion_context),
		    fiscal_year_start_month = COALESCE($4, fiscal_year_start_month),
		    fiscal_year_start_day = COALESCE($5, fiscal_year_start_day)
		WHERE id = $1 AND tenant_id = $6
		RETURNING id
	`, entityID, name, suggestionContext, fiscalYearStartMonth, fiscalYearStartDay, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Entity{}, ErrNotFound
		}
		return models.Entity{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *EntityStore) GetByID(ctx context.Context, entityID string) (models.Entity, error) {
	row := EntityRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+entityColumns+`
		FROM entities e
		WHERE e.id = $1
	`, entityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Entity{}, ErrNotFound
		}
		return models.Entity{}, err
	}
	return entityFromRow(row), nil
}

func (s *EntityStore) Delete(ctx context.Context, tenantID, entityID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM entities
		WHERE id = $1 AND tenant_id = $2
	`, entityID, tenantID)
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

func (s *EntityStore) GetRole(ctx context.Context, tenantID, userID, entityID string) (models.Role, error) {
	var role string
	err := s.db.GetContext(ctx, &role, `
		SELECT eu.role
		FROM entity_users eu
		JOIN entities e ON e.id = eu.entity_id
		WHERE eu.user_id = $1 AND eu.entity_id = $2 AND e.tenant_id = $3
	`, userID, entityID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return models.Role(role), nil
}

func entityFromRow(row EntityRow) models.Entity {
	return models.Entity{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		Name:                 row.Name,
		SuggestionContext:    row.SuggestionContext,
		FiscalYearStartMonth: row.FiscalYearStartMonth,
		FiscalYearStartDay:   row.FiscalYearStartDay,
		CreatedAt:            row.CreatedAt,
	}
}
