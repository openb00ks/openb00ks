package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type UserMFA struct {
	UserID             string          `db:"user_id"`
	Secret             string          `db:"secret"`
	Enabled            bool            `db:"enabled"`
	RecoveryCodeHashes json.RawMessage `db:"recovery_code_hashes"`
	CreatedAt          time.Time       `db:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at"`
}

type UserMFAStore struct {
	db *DB
}

func NewUserMFAStore(db *DB) *UserMFAStore {
	return &UserMFAStore{db: db}
}

func (s *UserMFAStore) GetByUserID(ctx context.Context, userID string) (UserMFA, error) {
	var row UserMFA
	err := s.db.GetContext(ctx, &row, `
		SELECT user_id, secret, enabled, recovery_code_hashes, created_at, updated_at
		FROM user_mfa
		WHERE user_id = $1
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserMFA{}, ErrNotFound
		}
		return UserMFA{}, err
	}
	return row, nil
}

func (s *UserMFAStore) UpsertEnrollment(ctx context.Context, userID, secret string, recoveryCodeHashes json.RawMessage) (UserMFA, error) {
	if len(recoveryCodeHashes) == 0 {
		recoveryCodeHashes = json.RawMessage(`[]`)
	}
	var row UserMFA
	err := s.db.GetContext(ctx, &row, `
		INSERT INTO user_mfa (user_id, secret, enabled, recovery_code_hashes)
		VALUES ($1, $2, false, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET secret = EXCLUDED.secret,
			enabled = false,
			recovery_code_hashes = EXCLUDED.recovery_code_hashes,
			updated_at = now()
		RETURNING user_id, secret, enabled, recovery_code_hashes, created_at, updated_at
	`, userID, secret, recoveryCodeHashes)
	if err != nil {
		return UserMFA{}, err
	}
	return row, nil
}

func (s *UserMFAStore) Enable(ctx context.Context, userID string) (UserMFA, error) {
	var row UserMFA
	err := s.db.GetContext(ctx, &row, `
		UPDATE user_mfa
		SET enabled = true,
			updated_at = now()
		WHERE user_id = $1
		RETURNING user_id, secret, enabled, recovery_code_hashes, created_at, updated_at
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserMFA{}, ErrNotFound
		}
		return UserMFA{}, err
	}
	return row, nil
}

func (s *UserMFAStore) Disable(ctx context.Context, userID string) (UserMFA, error) {
	var row UserMFA
	err := s.db.GetContext(ctx, &row, `
		UPDATE user_mfa
		SET secret = '',
			enabled = false,
			recovery_code_hashes = '[]'::jsonb,
			updated_at = now()
		WHERE user_id = $1
		RETURNING user_id, secret, enabled, recovery_code_hashes, created_at, updated_at
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserMFA{}, ErrNotFound
		}
		return UserMFA{}, err
	}
	return row, nil
}

func (s *UserMFAStore) SetRecoveryCodeHashes(ctx context.Context, userID string, recoveryCodeHashes json.RawMessage) (UserMFA, error) {
	if len(recoveryCodeHashes) == 0 {
		recoveryCodeHashes = json.RawMessage(`[]`)
	}
	var row UserMFA
	err := s.db.GetContext(ctx, &row, `
		UPDATE user_mfa
		SET recovery_code_hashes = $2,
			updated_at = now()
		WHERE user_id = $1
		RETURNING user_id, secret, enabled, recovery_code_hashes, created_at, updated_at
	`, userID, recoveryCodeHashes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserMFA{}, ErrNotFound
		}
		return UserMFA{}, err
	}
	return row, nil
}
