package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, email, passwordHash string, isAdmin bool) (models.User, error) {
	row := UserRow{}
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO users (email, password_hash, is_admin)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, default_tenant_id, is_admin, created_at
	`, email, passwordHash, isAdmin).StructScan(&row)
	if err != nil {
		return models.User{}, err
	}
	return userFromRow(row), nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (models.User, error) {
	row := UserRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT id, email, password_hash, default_tenant_id, is_admin, created_at
		FROM users
		WHERE email = $1
	`, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, ErrNotFound
		}
		return models.User{}, err
	}
	return userFromRow(row), nil
}

func (s *UserStore) GetByID(ctx context.Context, id string) (models.User, error) {
	row := UserRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT id, email, password_hash, default_tenant_id, is_admin, created_at
		FROM users
		WHERE id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, ErrNotFound
		}
		return models.User{}, err
	}
	return userFromRow(row), nil
}

func (s *UserStore) List(ctx context.Context, limit int) ([]models.User, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []UserRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT id, email, password_hash, default_tenant_id, is_admin, created_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	users := make([]models.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromRow(row))
	}
	return users, nil
}

func (s *UserStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM users`); err != nil {
		return 0, err
	}
	return count, nil
}

func userFromRow(row UserRow) models.User {
	var defaultTenant string
	if row.DefaultTenantID.Valid {
		defaultTenant = row.DefaultTenantID.String
	}
	return models.User{
		ID:              row.ID,
		Email:           row.Email,
		PasswordHash:    row.PasswordHash,
		DefaultTenantID: defaultTenant,
		IsAdmin:         row.IsAdmin,
		CreatedAt:       row.CreatedAt,
	}
}

func (s *UserStore) SetDefaultTenant(ctx context.Context, userID, tenantID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET default_tenant_id = $2
		WHERE id = $1
	`, userID, tenantID)
	return err
}
