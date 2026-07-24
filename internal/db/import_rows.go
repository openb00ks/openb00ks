package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const importRowColumns = "ir.id, ir.receipt_id, ir.entity_id, ir.row_index, ir.date, ir.vendor, ir.memo, ir.amount_cents, ir.direction, ir.account_id, ir.fingerprint, ir.status, ir.transaction_id, ir.raw_json, ir.created_at, ir.updated_at"

type ImportRowStore struct {
	db *DB
}

func NewImportRowStore(db *DB) *ImportRowStore {
	return &ImportRowStore{db: db}
}

func (s *ImportRowStore) ReplaceForReceipt(ctx context.Context, receiptID, entityID string, rows []models.ImportRow) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `DELETE FROM import_rows WHERE receipt_id = $1 AND transaction_id IS NULL`, receiptID); err != nil {
		return err
	}
	for _, row := range rows {
		var accountID sql.NullString
		if row.AccountID != "" {
			accountID = sql.NullString{String: row.AccountID, Valid: true}
		}
		if row.Status == "" {
			row.Status = "needs_review"
		}
		if len(row.RawJSON) == 0 {
			row.RawJSON = []byte(`[]`)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO import_rows (receipt_id, entity_id, row_index, date, vendor, memo, amount_cents, direction, account_id, fingerprint, status, raw_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (receipt_id, row_index)
			DO UPDATE SET
			  date = EXCLUDED.date,
			  vendor = EXCLUDED.vendor,
			  memo = EXCLUDED.memo,
			  amount_cents = EXCLUDED.amount_cents,
			  direction = EXCLUDED.direction,
			  account_id = COALESCE(import_rows.account_id, EXCLUDED.account_id),
			  fingerprint = EXCLUDED.fingerprint,
			  status = CASE WHEN import_rows.transaction_id IS NOT NULL THEN import_rows.status ELSE EXCLUDED.status END,
			  raw_json = EXCLUDED.raw_json,
			  updated_at = now()
		`, receiptID, entityID, row.RowIndex, row.Date, row.Vendor, nullString(row.Memo), row.AmountCents, row.Direction, accountID, row.Fingerprint, row.Status, row.RawJSON)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ImportRowStore) ListByReceiptID(ctx context.Context, receiptID string) ([]models.ImportRow, error) {
	rows := []ImportRowRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+importRowColumns+`
		FROM import_rows ir
		WHERE ir.receipt_id = $1
		ORDER BY ir.row_index
	`, receiptID); err != nil {
		return nil, err
	}
	out := make([]models.ImportRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, importRowFromRow(row))
	}
	return out, nil
}

func (s *ImportRowStore) GetByReceiptAndIndex(ctx context.Context, receiptID string, rowIndex int) (models.ImportRow, error) {
	row := ImportRowRow{}
	if err := s.db.GetContext(ctx, &row, `
		SELECT `+importRowColumns+`
		FROM import_rows ir
		WHERE ir.receipt_id = $1 AND ir.row_index = $2
	`, receiptID, rowIndex); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ImportRow{}, ErrNotFound
		}
		return models.ImportRow{}, err
	}
	return importRowFromRow(row), nil
}

func (s *ImportRowStore) UpdateAccount(ctx context.Context, receiptID string, rowIndex int, accountID string) (models.ImportRow, error) {
	var id string
	if err := s.db.GetContext(ctx, &id, `
		UPDATE import_rows
		SET account_id = $3, status = CASE WHEN status = 'posted' THEN status ELSE 'mapped' END, updated_at = now()
		WHERE receipt_id = $1 AND row_index = $2
		RETURNING id
	`, receiptID, rowIndex, accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ImportRow{}, ErrNotFound
		}
		return models.ImportRow{}, err
	}
	return s.getByID(ctx, id)
}

func (s *ImportRowStore) MarkPosted(ctx context.Context, id string, transactionID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE import_rows
		SET status = 'posted', transaction_id = $2, updated_at = now()
		WHERE id = $1 AND transaction_id IS NULL
	`, id, transactionID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrReceiptAlreadyAttached
	}
	return nil
}

func (s *ImportRowStore) getByID(ctx context.Context, id string) (models.ImportRow, error) {
	row := ImportRowRow{}
	if err := s.db.GetContext(ctx, &row, `
		SELECT `+importRowColumns+`
		FROM import_rows ir
		WHERE ir.id = $1
	`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ImportRow{}, ErrNotFound
		}
		return models.ImportRow{}, err
	}
	return importRowFromRow(row), nil
}

func importRowFromRow(row ImportRowRow) models.ImportRow {
	out := models.ImportRow{
		ID:          row.ID,
		ReceiptID:   row.ReceiptID,
		EntityID:    row.EntityID,
		RowIndex:    row.RowIndex,
		Date:        row.Date,
		Vendor:      row.Vendor,
		AmountCents: row.AmountCents,
		Direction:   row.Direction,
		Fingerprint: row.Fingerprint,
		Status:      row.Status,
		RawJSON:     row.RawJSON,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.Memo.Valid {
		out.Memo = row.Memo.String
	}
	if row.AccountID.Valid {
		out.AccountID = row.AccountID.String
	}
	if row.TransactionID.Valid {
		out.TransactionID = row.TransactionID.String
	}
	return out
}
