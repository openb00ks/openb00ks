package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const entityUserColumns = "eu.id, eu.user_id, eu.entity_id, eu.role, eu.created_at"

type MembershipStore struct {
	db *DB
}

func NewMembershipStore(db *DB) *MembershipStore {
	return &MembershipStore{db: db}
}

func (s *MembershipStore) List(ctx context.Context, entityID string, limit int) ([]models.EntityUser, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []EntityUserRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+entityUserColumns+`
		FROM entity_users eu
		WHERE eu.entity_id = $1
		ORDER BY eu.created_at DESC
		LIMIT $2
	`, entityID, limit)
	if err != nil {
		return nil, err
	}
	members := make([]models.EntityUser, 0, len(rows))
	for _, row := range rows {
		members = append(members, entityUserFromRow(row))
	}
	return members, nil
}

func (s *MembershipStore) Create(ctx context.Context, entityID, userID string, role models.Role) (models.EntityUser, error) {
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO entity_users (user_id, entity_id, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, entityID, string(role))
	if err != nil {
		return models.EntityUser{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *MembershipStore) UpdateRole(ctx context.Context, entityUserID string, role models.Role) (models.EntityUser, error) {
	var id string
	err := s.db.GetContext(ctx, &id, `
		UPDATE entity_users
		SET role = $1
		WHERE id = $2
		RETURNING id
	`, string(role), entityUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.EntityUser{}, ErrNotFound
		}
		return models.EntityUser{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *MembershipStore) GetByID(ctx context.Context, id string) (models.EntityUser, error) {
	row := EntityUserRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+entityUserColumns+`
		FROM entity_users eu
		WHERE eu.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.EntityUser{}, ErrNotFound
		}
		return models.EntityUser{}, err
	}
	return entityUserFromRow(row), nil
}

func (s *MembershipStore) Delete(ctx context.Context, entityUserID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM entity_users
		WHERE id = $1
	`, entityUserID)
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

func entityUserFromRow(row EntityUserRow) models.EntityUser {
	return models.EntityUser{
		ID:        row.ID,
		UserID:    row.UserID,
		EntityID:  row.EntityID,
		Role:      models.Role(row.Role),
		CreatedAt: row.CreatedAt,
	}
}
