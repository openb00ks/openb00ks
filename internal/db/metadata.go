package db

import (
	"context"
)

type ReceiptMetadataStore struct {
	db *DB
}

const (
	suggestionContextKey = "suggestion_context"
	legacyContextKey     = "context"
)

func NewReceiptMetadataStore(db *DB) *ReceiptMetadataStore {
	return &ReceiptMetadataStore{db: db}
}

func (s *ReceiptMetadataStore) UpsertContext(ctx context.Context, receiptID, contextText string) error {
	return s.UpsertSuggestionContext(ctx, receiptID, contextText)
}

func (s *ReceiptMetadataStore) UpsertSuggestionContext(ctx context.Context, receiptID, contextText string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO receipt_metadata (receipt_id, data_json)
		VALUES ($1, jsonb_build_object($3::text, to_jsonb($2::text)))
		ON CONFLICT (receipt_id) DO UPDATE
		SET data_json = jsonb_set(receipt_metadata.data_json, ARRAY[$3::text], to_jsonb($2::text), true)
	`, receiptID, contextText, suggestionContextKey)
	return err
}

func (s *ReceiptMetadataStore) GetContext(ctx context.Context, receiptID string) (string, error) {
	return s.GetSuggestionContext(ctx, receiptID)
}

func (s *ReceiptMetadataStore) GetSuggestionContext(ctx context.Context, receiptID string) (string, error) {
	var contextText string
	if err := s.db.GetContext(ctx, &contextText, `
		SELECT COALESCE(data_json->>$2, data_json->>$3, '')
		FROM receipt_metadata
		WHERE receipt_id = $1
	`, receiptID, suggestionContextKey, legacyContextKey); err != nil {
		return "", err
	}
	return contextText, nil
}

type MileageMetadataStore struct {
	db *DB
}

func NewMileageMetadataStore(db *DB) *MileageMetadataStore {
	return &MileageMetadataStore{db: db}
}

func (s *MileageMetadataStore) UpsertContext(ctx context.Context, mileageID, contextText string) error {
	return s.UpsertSuggestionContext(ctx, mileageID, contextText)
}

func (s *MileageMetadataStore) UpsertSuggestionContext(ctx context.Context, mileageID, contextText string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO mileage_metadata (mileage_id, data_json)
		VALUES ($1, jsonb_build_object($3::text, to_jsonb($2::text)))
		ON CONFLICT (mileage_id) DO UPDATE
		SET data_json = jsonb_set(mileage_metadata.data_json, ARRAY[$3::text], to_jsonb($2::text), true)
	`, mileageID, contextText, suggestionContextKey)
	return err
}

func (s *MileageMetadataStore) GetContext(ctx context.Context, mileageID string) (string, error) {
	return s.GetSuggestionContext(ctx, mileageID)
}

func (s *MileageMetadataStore) GetSuggestionContext(ctx context.Context, mileageID string) (string, error) {
	var contextText string
	if err := s.db.GetContext(ctx, &contextText, `
		SELECT COALESCE(data_json->>$2, data_json->>$3, '')
		FROM mileage_metadata
		WHERE mileage_id = $1
	`, mileageID, suggestionContextKey, legacyContextKey); err != nil {
		return "", err
	}
	return contextText, nil
}
