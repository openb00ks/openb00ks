package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type UsedTOTPStepStore struct {
	db *DB
}

func NewUsedTOTPStepStore(db *DB) *UsedTOTPStepStore {
	return &UsedTOTPStepStore{db: db}
}

// MarkUsed inserts (userID, step) to record that the code for this step has
// been consumed.  Returns ErrConflict if the step was already used (replay).
// Old entries (>2 leeway windows) are pruned in the same statement to avoid
// unbounded table growth.
func (s *UsedTOTPStepStore) MarkUsed(ctx context.Context, userID string, step int64, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx, `
		WITH pruned AS (
			DELETE FROM used_totp_steps
			WHERE user_id = $1 AND step < $3
		)
		INSERT INTO used_totp_steps (user_id, step, used_at)
		VALUES ($1, $2, $4)
	`, userID, step, step-2, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return err
	}
	return nil
}
