package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

const draftTransactionColumns = "dt.id, dt.receipt_id, dt.entity_id, dt.date, dt.memo, dt.created_at, dt.updated_at"
const draftEntryColumns = "de.id, de.draft_transaction_id, de.account_id, de.debit_cents, de.credit_cents"

type DraftStore struct {
	db *DB
}

func NewDraftStore(db *DB) *DraftStore {
	return &DraftStore{db: db}
}

func (s *DraftStore) GetByReceiptID(ctx context.Context, receiptID string) (models.DraftTransaction, error) {
	dt := DraftTransactionRow{}
	err := s.db.GetContext(ctx, &dt, `
		SELECT `+draftTransactionColumns+`
		FROM draft_transactions dt
		WHERE dt.receipt_id = $1
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.DraftTransaction{}, ErrNotFound
		}
		return models.DraftTransaction{}, err
	}

	rows := []DraftEntryRow{}
	err = s.db.SelectContext(ctx, &rows, `
		SELECT `+draftEntryColumns+`
		FROM draft_entries de
		WHERE de.draft_transaction_id = $1
		ORDER BY de.id
	`, dt.ID)
	if err != nil {
		return models.DraftTransaction{}, err
	}
	return draftFromRows(dt, rows), nil
}

func (s *DraftStore) EnsureForReceipt(ctx context.Context, receiptID string) (models.DraftTransaction, error) {
	draft, err := s.GetByReceiptID(ctx, receiptID)
	if err == nil {
		return draft, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return models.DraftTransaction{}, err
	}
	var id string
	err = s.db.GetContext(ctx, &id, `
		INSERT INTO draft_transactions (receipt_id, entity_id, date)
		SELECT r.id, r.entity_id, CURRENT_DATE
		FROM receipts r
		WHERE r.id = $1
		RETURNING id
	`, receiptID)
	if err != nil {
		return models.DraftTransaction{}, err
	}
	return s.GetByReceiptID(ctx, receiptID)
}

func (s *DraftStore) SetEntriesByReceipt(ctx context.Context, receiptID string, entries []models.DraftEntry) error {
	draft, err := s.GetByReceiptID(ctx, receiptID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	_, err = tx.ExecContext(ctx, `
		DELETE FROM draft_entries
		WHERE draft_transaction_id = $1
	`, draft.ID)
	if err != nil {
		return err
	}
	for _, e := range entries {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO draft_entries (draft_transaction_id, account_id, debit_cents, credit_cents)
			VALUES ($1, $2, $3, $4)
		`, draft.ID, e.AccountID, e.DebitCents, e.CreditCents)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *DraftStore) UpdateDraft(ctx context.Context, receiptID string, date time.Time, memo string, entries []models.DraftEntry) (models.DraftTransaction, error) {
	if err := validateBalanced(entries); err != nil {
		return models.DraftTransaction{}, err
	}
	draft, err := s.GetByReceiptID(ctx, receiptID)
	if err != nil {
		return models.DraftTransaction{}, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.DraftTransaction{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx, `
		UPDATE draft_transactions
		SET date = $2, memo = $3, updated_at = now()
		WHERE id = $1
	`, draft.ID, date, nullString(memo))
	if err != nil {
		return models.DraftTransaction{}, err
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM draft_entries
		WHERE draft_transaction_id = $1
	`, draft.ID)
	if err != nil {
		return models.DraftTransaction{}, err
	}

	for _, e := range entries {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO draft_entries (draft_transaction_id, account_id, debit_cents, credit_cents)
			VALUES ($1, $2, $3, $4)
		`, draft.ID, e.AccountID, e.DebitCents, e.CreditCents)
		if err != nil {
			return models.DraftTransaction{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.DraftTransaction{}, err
	}

	return s.GetByReceiptID(ctx, receiptID)
}

func draftFromRows(dt DraftTransactionRow, entries []DraftEntryRow) models.DraftTransaction {
	out := models.DraftTransaction{
		ID:        dt.ID,
		ReceiptID: dt.ReceiptID,
		EntityID:  dt.EntityID,
		Date:      dt.Date,
		CreatedAt: dt.CreatedAt,
		UpdatedAt: dt.UpdatedAt,
	}
	if dt.Memo.Valid {
		out.Memo = dt.Memo.String
	}
	out.Entries = make([]models.DraftEntry, 0, len(entries))
	for _, row := range entries {
		out.Entries = append(out.Entries, models.DraftEntry{
			ID:                 row.ID,
			DraftTransactionID: row.DraftTransactionID,
			AccountID:          row.AccountID,
			DebitCents:         row.DebitCents,
			CreditCents:        row.CreditCents,
		})
	}
	return out
}
