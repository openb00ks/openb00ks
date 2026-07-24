package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const receiptOCRColumns = "ro.id, ro.receipt_id, ro.provider, ro.status, ro.raw_text, ro.data_json, ro.error, ro.input_hash, ro.run_version, ro.created_at"

type ReceiptOCRStore struct {
	db *DB
}

func NewReceiptOCRStore(db *DB) *ReceiptOCRStore {
	return &ReceiptOCRStore{db: db}
}

func (s *ReceiptOCRStore) Create(ctx context.Context, ocr models.ReceiptOCR) (models.ReceiptOCR, error) {
	var rawText sql.NullString
	if ocr.RawText != "" {
		rawText = sql.NullString{String: ocr.RawText, Valid: true}
	}
	var errMsg sql.NullString
	if ocr.Error != "" {
		errMsg = sql.NullString{String: ocr.Error, Valid: true}
	}
	var inputHash sql.NullString
	if ocr.InputHash != "" {
		inputHash = sql.NullString{String: ocr.InputHash, Valid: true}
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO receipt_ocr (receipt_id, provider, status, raw_text, data_json, error, input_hash, run_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, ocr.ReceiptID, ocr.Provider, ocr.Status, rawText, ocr.DataJSON, errMsg, inputHash, ocr.RunVersion)
	if err != nil {
		return models.ReceiptOCR{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *ReceiptOCRStore) GetByID(ctx context.Context, id string) (models.ReceiptOCR, error) {
	row := ReceiptOCRRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+receiptOCRColumns+`
		FROM receipt_ocr ro
		WHERE ro.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ReceiptOCR{}, ErrNotFound
		}
		return models.ReceiptOCR{}, err
	}
	return receiptOCRFromRow(row), nil
}

func (s *ReceiptOCRStore) ListByReceiptID(ctx context.Context, receiptID string, limit int) ([]models.ReceiptOCR, error) {
	if limit <= 0 {
		limit = 50
	}
	rows := []ReceiptOCRRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+receiptOCRColumns+`
		FROM receipt_ocr ro
		WHERE ro.receipt_id = $1
		ORDER BY ro.created_at DESC
		LIMIT $2
	`, receiptID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ReceiptOCR, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptOCRFromRow(row))
	}
	return out, nil
}

func (s *ReceiptOCRStore) LatestByReceiptID(ctx context.Context, receiptID string) (models.ReceiptOCR, error) {
	row := ReceiptOCRRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+receiptOCRColumns+`
		FROM receipt_ocr ro
		WHERE ro.receipt_id = $1
		ORDER BY ro.created_at DESC
		LIMIT 1
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ReceiptOCR{}, ErrNotFound
		}
		return models.ReceiptOCR{}, err
	}
	return receiptOCRFromRow(row), nil
}

func receiptOCRFromRow(row ReceiptOCRRow) models.ReceiptOCR {
	ocr := models.ReceiptOCR{
		ID:         row.ID,
		ReceiptID:  row.ReceiptID,
		Provider:   row.Provider,
		Status:     row.Status,
		DataJSON:   row.DataJSON,
		RunVersion: row.RunVersion,
		CreatedAt:  row.CreatedAt,
	}
	if row.RawText.Valid {
		ocr.RawText = row.RawText.String
	}
	if row.Error.Valid {
		ocr.Error = row.Error.String
	}
	if row.InputHash.Valid {
		ocr.InputHash = row.InputHash.String
	}
	return ocr
}
