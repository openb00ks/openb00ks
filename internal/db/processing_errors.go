package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

const processingErrorColumns = "pe.id, pe.entity_id, pe.receipt_id, pe.mileage_id, pe.stage, pe.error, pe.created_at, pe.resolved_at, pe.resolution_note"

type ProcessingErrorStore struct {
	db *DB
}

func NewProcessingErrorStore(db *DB) *ProcessingErrorStore {
	return &ProcessingErrorStore{db: db}
}

func (s *ProcessingErrorStore) Create(ctx context.Context, errEvent models.ProcessingError) (models.ProcessingError, error) {
	var receiptID sql.NullString
	if errEvent.ReceiptID != "" {
		receiptID = sql.NullString{String: errEvent.ReceiptID, Valid: true}
	}
	var mileageID sql.NullString
	if errEvent.MileageID != "" {
		mileageID = sql.NullString{String: errEvent.MileageID, Valid: true}
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO processing_errors (entity_id, receipt_id, mileage_id, stage, error)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, errEvent.EntityID, receiptID, mileageID, errEvent.Stage, errEvent.Error)
	if err != nil {
		return models.ProcessingError{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *ProcessingErrorStore) GetByID(ctx context.Context, id string) (models.ProcessingError, error) {
	row := ProcessingErrorRow{}
	if err := s.db.GetContext(ctx, &row, `
		SELECT `+processingErrorColumns+`
		FROM processing_errors pe
		WHERE pe.id = $1
	`, id); err != nil {
		return models.ProcessingError{}, err
	}
	return processingErrorFromRow(row), nil
}

func (s *ProcessingErrorStore) ListByReceiptID(ctx context.Context, receiptID string, limit int) ([]models.ProcessingError, error) {
	if limit <= 0 {
		limit = 50
	}
	rows := []ProcessingErrorRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+processingErrorColumns+`
		FROM processing_errors pe
		WHERE pe.receipt_id = $1
		ORDER BY pe.created_at DESC
		LIMIT $2
	`, receiptID, limit); err != nil {
		return nil, err
	}
	out := make([]models.ProcessingError, 0, len(rows))
	for _, row := range rows {
		out = append(out, processingErrorFromRow(row))
	}
	return out, nil
}

func (s *ProcessingErrorStore) List(ctx context.Context, resolved *bool, limit, offset int) ([]models.ProcessingError, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []interface{}{limit, offset}
	where := ""
	if resolved != nil {
		if *resolved {
			where = "WHERE pe.resolved_at IS NOT NULL"
		} else {
			where = "WHERE pe.resolved_at IS NULL"
		}
	}
	rows := []ProcessingErrorRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+processingErrorColumns+`
		FROM processing_errors pe
		`+where+`
		ORDER BY pe.created_at DESC
		LIMIT $1 OFFSET $2
	`, args...); err != nil {
		return nil, err
	}
	out := make([]models.ProcessingError, 0, len(rows))
	for _, row := range rows {
		out = append(out, processingErrorFromRow(row))
	}
	return out, nil
}

func (s *ProcessingErrorStore) Resolve(ctx context.Context, id, note string, resolvedAt time.Time) (models.ProcessingError, error) {
	var noteVal sql.NullString
	if note != "" {
		noteVal = sql.NullString{String: note, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE processing_errors
		SET resolved_at = $2, resolution_note = $3
		WHERE id = $1 AND resolved_at IS NULL
	`, id, resolvedAt, noteVal)
	if err != nil {
		return models.ProcessingError{}, err
	}
	return s.GetByID(ctx, id)
}

func processingErrorFromRow(row ProcessingErrorRow) models.ProcessingError {
	out := models.ProcessingError{
		ID:        row.ID,
		EntityID:  row.EntityID,
		Stage:     row.Stage,
		Error:     row.Error,
		CreatedAt: row.CreatedAt,
	}
	if row.ReceiptID.Valid {
		out.ReceiptID = row.ReceiptID.String
	}
	if row.MileageID.Valid {
		out.MileageID = row.MileageID.String
	}
	if row.ResolvedAt.Valid {
		t := row.ResolvedAt.Time
		out.ResolvedAt = &t
	}
	if row.ResolutionNote.Valid {
		out.ResolutionNote = row.ResolutionNote.String
	}
	return out
}
