package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const receiptSuggestionColumns = "rs.id, rs.receipt_id, rs.provider, rs.model, rs.status, rs.prompt_json, rs.raw_response, rs.parsed_json, rs.confidence, rs.error, rs.input_hash, rs.run_version, rs.created_at, rs.prompt_tokens, rs.completion_tokens, rs.total_tokens, rs.cost_cents"

type ReceiptSuggestionStore struct {
	db *DB
}

func NewReceiptSuggestionStore(db *DB) *ReceiptSuggestionStore {
	return &ReceiptSuggestionStore{db: db}
}

func (s *ReceiptSuggestionStore) Create(ctx context.Context, suggestion models.ReceiptSuggestion) (models.ReceiptSuggestion, error) {
	var confidence sql.NullFloat64
	if suggestion.Confidence > 0 {
		confidence = sql.NullFloat64{Float64: suggestion.Confidence, Valid: true}
	}
	var errMsg sql.NullString
	if suggestion.Error != "" {
		errMsg = sql.NullString{String: suggestion.Error, Valid: true}
	}
	var inputHash sql.NullString
	if suggestion.InputHash != "" {
		inputHash = sql.NullString{String: suggestion.InputHash, Valid: true}
	}
	if suggestion.TotalTokens == 0 && (suggestion.PromptTokens > 0 || suggestion.CompletionTokens > 0) {
		suggestion.TotalTokens = suggestion.PromptTokens + suggestion.CompletionTokens
	}
	var promptTokens sql.NullInt64
	if suggestion.PromptTokens > 0 {
		promptTokens = sql.NullInt64{Int64: suggestion.PromptTokens, Valid: true}
	}
	var completionTokens sql.NullInt64
	if suggestion.CompletionTokens > 0 {
		completionTokens = sql.NullInt64{Int64: suggestion.CompletionTokens, Valid: true}
	}
	var totalTokens sql.NullInt64
	if suggestion.TotalTokens > 0 {
		totalTokens = sql.NullInt64{Int64: suggestion.TotalTokens, Valid: true}
	}
	var costCents sql.NullInt64
	if suggestion.CostCents > 0 {
		costCents = sql.NullInt64{Int64: suggestion.CostCents, Valid: true}
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO receipt_suggestions (receipt_id, provider, model, status, prompt_json, raw_response, parsed_json, confidence, error, input_hash, run_version, prompt_tokens, completion_tokens, total_tokens, cost_cents)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id
	`, suggestion.ReceiptID, suggestion.Provider, suggestion.Model, suggestion.Status, suggestion.PromptJSON, suggestion.RawJSON, suggestion.ParsedJSON, confidence, errMsg, inputHash, suggestion.RunVersion, promptTokens, completionTokens, totalTokens, costCents)
	if err != nil {
		return models.ReceiptSuggestion{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *ReceiptSuggestionStore) GetByID(ctx context.Context, id string) (models.ReceiptSuggestion, error) {
	row := ReceiptSuggestionRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+receiptSuggestionColumns+`
		FROM receipt_suggestions rs
		WHERE rs.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ReceiptSuggestion{}, ErrNotFound
		}
		return models.ReceiptSuggestion{}, err
	}
	return receiptSuggestionFromRow(row), nil
}

func (s *ReceiptSuggestionStore) ListByReceiptID(ctx context.Context, receiptID string, limit int) ([]models.ReceiptSuggestion, error) {
	if limit <= 0 {
		limit = 50
	}
	rows := []ReceiptSuggestionRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+receiptSuggestionColumns+`
		FROM receipt_suggestions rs
		WHERE rs.receipt_id = $1
		ORDER BY rs.created_at DESC
		LIMIT $2
	`, receiptID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ReceiptSuggestion, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptSuggestionFromRow(row))
	}
	return out, nil
}

func (s *ReceiptSuggestionStore) LatestByReceiptID(ctx context.Context, receiptID string) (models.ReceiptSuggestion, error) {
	row := ReceiptSuggestionRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+receiptSuggestionColumns+`
		FROM receipt_suggestions rs
		WHERE rs.receipt_id = $1
		ORDER BY rs.created_at DESC
		LIMIT 1
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ReceiptSuggestion{}, ErrNotFound
		}
		return models.ReceiptSuggestion{}, err
	}
	return receiptSuggestionFromRow(row), nil
}

func receiptSuggestionFromRow(row ReceiptSuggestionRow) models.ReceiptSuggestion {
	suggestion := models.ReceiptSuggestion{
		ID:         row.ID,
		ReceiptID:  row.ReceiptID,
		Provider:   row.Provider,
		Model:      row.Model,
		Status:     row.Status,
		PromptJSON: row.PromptJSON,
		RawJSON:    row.RawJSON,
		ParsedJSON: row.ParsedJSON,
		RunVersion: row.RunVersion,
		CreatedAt:  row.CreatedAt,
	}
	if row.Confidence.Valid {
		suggestion.Confidence = row.Confidence.Float64
	}
	if row.Error.Valid {
		suggestion.Error = row.Error.String
	}
	if row.InputHash.Valid {
		suggestion.InputHash = row.InputHash.String
	}
	if row.PromptTokens.Valid {
		suggestion.PromptTokens = row.PromptTokens.Int64
	}
	if row.CompletionTokens.Valid {
		suggestion.CompletionTokens = row.CompletionTokens.Int64
	}
	if row.TotalTokens.Valid {
		suggestion.TotalTokens = row.TotalTokens.Int64
	}
	if row.CostCents.Valid {
		suggestion.CostCents = row.CostCents.Int64
	}
	return suggestion
}
