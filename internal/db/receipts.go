package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const receiptColumns = "r.id, r.entity_id, r.storage_key, r.content_type, r.size_bytes, r.status, r.kind, r.total_cents, r.uploaded_at, r.attached_at, r.original_name, r.resolved_vendor_id::text AS resolved_vendor_id, r.resolved_vendor_raw, r.ai_summary"

type ReceiptStore struct {
	db *DB
}

func NewReceiptStore(db *DB) *ReceiptStore {
	return &ReceiptStore{db: db}
}

func (s *ReceiptStore) Create(ctx context.Context, entityID, storageKey, contentType, status, kind, originalName string, sizeBytes int64, totalCents int64) (models.Receipt, error) {
	var total sql.NullInt64
	if totalCents > 0 {
		total = sql.NullInt64{Int64: totalCents, Valid: true}
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, kind, original_name, total_cents)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, entityID, storageKey, contentType, sizeBytes, status, kind, originalName, total)
	if err != nil {
		return models.Receipt{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *ReceiptStore) GetByID(ctx context.Context, id string) (models.Receipt, error) {
	row := ReceiptRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+receiptColumns+`
		FROM receipts r
		WHERE r.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Receipt{}, ErrNotFound
		}
		return models.Receipt{}, err
	}
	return receiptFromRow(row), nil
}

func (s *ReceiptStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE receipts
		SET status = $2
		WHERE id = $1
	`, id, status)
	return err
}

func (s *ReceiptStore) GetEntityID(ctx context.Context, id string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `
		SELECT r.entity_id
		FROM receipts r
		WHERE r.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return entityID, nil
}

func receiptFromRow(row ReceiptRow) models.Receipt {
	receipt := models.Receipt{
		ID:          row.ID,
		EntityID:    row.EntityID,
		StorageKey:  row.StorageKey,
		ContentType: row.ContentType,
		SizeBytes:   row.SizeBytes,
		Status:      row.Status,
		Kind:        row.Kind,
		UploadedAt:  row.UploadedAt,
	}
	if row.TotalCents.Valid {
		receipt.TotalCents = row.TotalCents.Int64
	}
	if row.AttachedAt.Valid {
		t := row.AttachedAt.Time
		receipt.AttachedAt = &t
	}
	if row.OriginalName.Valid {
		receipt.OriginalName = row.OriginalName.String
	}
	receipt.ResolvedVendorID = row.ResolvedVendorID.String
	receipt.ResolvedVendorRaw = row.ResolvedVendorRaw.String
	if len(row.AISummary) > 0 {
		var summary models.ReceiptAISummary
		if err := json.Unmarshal(row.AISummary, &summary); err == nil && summary.HasContent() {
			receipt.AISummary = &summary
		}
	}
	return receipt
}

// SetAISummary persists the pipeline's display summary (vendor + account, confidence/reason) for a
// receipt so the review UI can explain the suggestion. A nil summary clears it.
func (s *ReceiptStore) SetAISummary(ctx context.Context, receiptID string, summary *models.ReceiptAISummary) error {
	var payload []byte
	if summary != nil && summary.HasContent() {
		b, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		payload = b
	}
	_, err := s.db.ExecContext(ctx, `UPDATE receipts SET ai_summary = $2 WHERE id = $1`, receiptID, payload)
	return err
}

// UpdateResolvedVendorID re-points a receipt at a different vendor (the reviewer correcting a mis-matched
// vendor), leaving the raw string intact so the correction still trains the right vendor on post. A blank
// vendorID clears the linkage. Returns ErrNotFound if the receipt doesn't exist.
func (s *ReceiptStore) UpdateResolvedVendorID(ctx context.Context, receiptID, vendorID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE receipts SET resolved_vendor_id = NULLIF($2, '')::uuid WHERE id = $1`, receiptID, vendorID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetResolvedVendor records the vendor the pipeline resolved for a receipt (and the raw receipt string it
// matched), so a later post can feed the human's account choice back to that vendor. Best-effort: a blank
// vendorID clears the linkage.
func (s *ReceiptStore) SetResolvedVendor(ctx context.Context, receiptID, vendorID, rawVendor string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE receipts SET resolved_vendor_id = NULLIF($2, '')::uuid, resolved_vendor_raw = NULLIF($3, '')
		WHERE id = $1
	`, receiptID, vendorID, rawVendor)
	return err
}
