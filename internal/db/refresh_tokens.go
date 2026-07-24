package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

const refreshTokenColumns = "rt.id, rt.user_id, rt.tenant_id, rt.token_hash, rt.expires_at, rt.created_at, rt.last_used_at, rt.revoked_at"

type RefreshTokenStore struct {
	db *DB
}

func NewRefreshTokenStore(db *DB) *RefreshTokenStore {
	return &RefreshTokenStore{db: db}
}

func (s *RefreshTokenStore) Create(ctx context.Context, userID, tenantID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
	if s == nil || s.db == nil {
		return models.RefreshToken{}, ErrUnavailable
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, tenantID, tokenHash, expiresAt)
	if err != nil {
		return models.RefreshToken{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *RefreshTokenStore) GetByID(ctx context.Context, id string) (models.RefreshToken, error) {
	if s == nil || s.db == nil {
		return models.RefreshToken{}, ErrUnavailable
	}
	var row RefreshTokenRow
	err := s.db.GetContext(ctx, &row, `
		SELECT `+refreshTokenColumns+`
		FROM refresh_tokens rt
		WHERE rt.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.RefreshToken{}, ErrNotFound
		}
		return models.RefreshToken{}, err
	}
	return refreshTokenFromRow(row), nil
}

func (s *RefreshTokenStore) GetByHash(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
	if s == nil || s.db == nil {
		return models.RefreshToken{}, ErrUnavailable
	}
	var row RefreshTokenRow
	err := s.db.GetContext(ctx, &row, `
		SELECT `+refreshTokenColumns+`
		FROM refresh_tokens rt
		WHERE rt.token_hash = $1
	`, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.RefreshToken{}, ErrNotFound
		}
		return models.RefreshToken{}, err
	}
	return refreshTokenFromRow(row), nil
}

func (s *RefreshTokenStore) Revoke(ctx context.Context, id string, usedAt time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, last_used_at = $2
		WHERE id = $1
	`, id, usedAt)
	return err
}

// RevokeIfActive atomically revokes the token only when it has not already been
// revoked.  Returns (true, nil) on success, (false, nil) when the token was
// already revoked (concurrent request), or (false, err) on a DB error.
func (s *RefreshTokenStore) RevokeIfActive(ctx context.Context, id string, revokedAt time.Time) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, last_used_at = $2
		WHERE id = $1 AND revoked_at IS NULL
	`, id, revokedAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (s *RefreshTokenStore) Touch(ctx context.Context, id string, usedAt time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET last_used_at = $2
		WHERE id = $1
	`, id, usedAt)
	return err
}

func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID string, revokedAt time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, last_used_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID, revokedAt)
	return err
}

func refreshTokenFromRow(row RefreshTokenRow) models.RefreshToken {
	var lastUsed *time.Time
	if row.LastUsedAt.Valid {
		lastUsed = &row.LastUsedAt.Time
	}
	var revoked *time.Time
	if row.RevokedAt.Valid {
		revoked = &row.RevokedAt.Time
	}
	return models.RefreshToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TenantID:   row.TenantID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		CreatedAt:  row.CreatedAt,
		LastUsedAt: lastUsed,
		RevokedAt:  revoked,
	}
}
