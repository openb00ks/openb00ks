package db

import (
	"context"
	"strconv"

	"github.com/openb00ks/openb00ks/internal/models"
)

func (s *ReceiptStore) List(ctx context.Context, entityID string, status string, limit int) ([]models.Receipt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []ReceiptRow{}
	query := `
		SELECT ` + receiptColumns + `
		FROM receipts r
		WHERE r.entity_id = $1
	`
	args := []interface{}{entityID}
	if status != "" {
		args = append(args, status)
		query += " AND r.status = $" + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	query += " ORDER BY r.uploaded_at DESC LIMIT $" + strconv.Itoa(len(args))

	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]models.Receipt, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptFromRow(row))
	}
	return out, nil
}

func (s *ReceiptStore) ListByKind(ctx context.Context, entityID, kind, status string, limit int) ([]models.Receipt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []ReceiptRow{}
	query := `
		SELECT ` + receiptColumns + `
		FROM receipts r
		WHERE r.entity_id = $1 AND r.kind = $2
	`
	args := []interface{}{entityID, kind}
	if status != "" {
		args = append(args, status)
		query += " AND r.status = $" + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	query += " ORDER BY r.uploaded_at DESC LIMIT $" + strconv.Itoa(len(args))

	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]models.Receipt, 0, len(rows))
	for _, row := range rows {
		out = append(out, receiptFromRow(row))
	}
	return out, nil
}
