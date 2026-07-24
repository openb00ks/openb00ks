package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const receiptJobIDColumn = "j.id"
const receiptJobColumns = "j.id, j.receipt_id, j.stage, j.status, j.attempts, j.max_attempts, j.locked_until, j.locked_by, j.last_error, j.created_at, j.updated_at"

type ReceiptJobStore struct {
	db *DB
}

func NewReceiptJobStore(db *DB) *ReceiptJobStore {
	return &ReceiptJobStore{db: db}
}

func (s *ReceiptJobStore) GetIDByReceiptID(ctx context.Context, receiptID string) (string, error) {
	var jobID string
	err := s.db.GetContext(ctx, &jobID, `
		SELECT `+receiptJobIDColumn+`
		FROM receipt_jobs j
		WHERE j.receipt_id = $1
		ORDER BY j.created_at DESC
		LIMIT 1
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return jobID, nil
}

func (s *ReceiptJobStore) ListByReceiptID(ctx context.Context, receiptID string, limit int) ([]models.ReceiptJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows := []ReceiptJobRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+receiptJobColumns+`
		FROM receipt_jobs j
		WHERE j.receipt_id = $1
		ORDER BY j.created_at DESC
		LIMIT $2
	`, receiptID, limit); err != nil {
		return nil, err
	}
	out := make([]models.ReceiptJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptJobFromRow(row))
	}
	return out, nil
}

func (s *ReceiptJobStore) List(ctx context.Context, status, stage string, limit, offset int) ([]models.ReceiptJob, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []interface{}{limit, offset}
	where := ""
	if status != "" && stage != "" {
		where = "WHERE j.status = $3 AND j.stage = $4"
		args = append(args, status, stage)
	} else if status != "" {
		where = "WHERE j.status = $3"
		args = append(args, status)
	} else if stage != "" {
		where = "WHERE j.stage = $3"
		args = append(args, stage)
	}
	rows := []ReceiptJobRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+receiptJobColumns+`
		FROM receipt_jobs j
		`+where+`
		ORDER BY j.updated_at DESC
		LIMIT $1 OFFSET $2
	`, args...); err != nil {
		return nil, err
	}
	out := make([]models.ReceiptJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptJobFromRow(row))
	}
	return out, nil
}

func receiptJobFromRow(row ReceiptJobRow) models.ReceiptJob {
	job := models.ReceiptJob{
		ID:          row.ID,
		ReceiptID:   row.ReceiptID,
		Stage:       row.Stage,
		Status:      row.Status,
		Attempts:    row.Attempts,
		MaxAttempts: row.MaxAttempts,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.LockedUntil.Valid {
		t := row.LockedUntil.Time
		job.LockedUntil = &t
	}
	if row.LockedBy.Valid {
		job.LockedBy = row.LockedBy.String
	}
	if row.LastError.Valid {
		job.LastError = row.LastError.String
	}
	return job
}
