package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/openb00ks/openb00ks/internal/models"
)

const transactionColumns = "t.id, t.entity_id, t.date, t.memo, t.created_at"
const entryColumns = "e.id, e.transaction_id, e.account_id, e.debit_cents, e.credit_cents"

type TransactionStore struct {
	db *DB
}

func NewTransactionStore(db *DB) *TransactionStore {
	return &TransactionStore{db: db}
}

type TransactionWithEntries struct {
	Transaction models.Transaction
	Entries     []models.Entry
}

func (s *TransactionStore) Create(ctx context.Context, entityID string, date time.Time, memo string, receiptID string, entries []models.DraftEntry) (models.Transaction, []models.Entry, error) {
	if err := validateBalanced(entries); err != nil {
		return models.Transaction{}, nil, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Transaction{}, nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := s.validateEntriesForEntity(ctx, tx, entityID, entries); err != nil {
		return models.Transaction{}, nil, err
	}
	if receiptID != "" {
		if err := s.lockReceiptForAttach(ctx, tx, receiptID, entityID); err != nil {
			return models.Transaction{}, nil, err
		}
	}

	var receipt sql.NullString
	if receiptID != "" {
		receipt = sql.NullString{String: receiptID, Valid: true}
	}
	var trID string
	err = tx.GetContext(ctx, &trID, `
		INSERT INTO transactions (entity_id, date, memo, receipt_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, entityID, date, nullString(memo), receipt)
	if err != nil {
		return models.Transaction{}, nil, err
	}

	outEntries := make([]models.Entry, 0, len(entries))
	for _, entry := range entries {
		var entryID string
		err = tx.GetContext(ctx, &entryID, `
			INSERT INTO entries (transaction_id, account_id, debit_cents, credit_cents)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, trID, entry.AccountID, entry.DebitCents, entry.CreditCents)
		if err != nil {
			return models.Transaction{}, nil, err
		}
		outEntries = append(outEntries, models.Entry{
			ID:            entryID,
			TransactionID: trID,
			AccountID:     entry.AccountID,
			DebitCents:    entry.DebitCents,
			CreditCents:   entry.CreditCents,
		})
	}

	if receiptID != "" {
		if err := attachReceipt(ctx, tx, receiptID); err != nil {
			return models.Transaction{}, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Transaction{}, nil, err
	}

	tr, err := s.GetByID(ctx, trID)
	if err != nil {
		return models.Transaction{}, nil, err
	}
	return tr, outEntries, nil
}

func (s *TransactionStore) GetByID(ctx context.Context, id string) (models.Transaction, error) {
	row := TransactionRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+transactionColumns+`
		FROM transactions t
		WHERE t.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Transaction{}, ErrNotFound
		}
		return models.Transaction{}, err
	}
	return transactionFromRow(row), nil
}

func (s *TransactionStore) List(ctx context.Context, entityID string, start, end *time.Time, limit int) ([]TransactionWithEntries, error) {
	if limit <= 0 {
		limit = 100
	}

	rows := []TransactionRow{}
	query := `
		SELECT ` + transactionColumns + `
		FROM transactions t
		WHERE t.entity_id = $1
	`
	args := []interface{}{entityID}
	if start != nil {
		args = append(args, *start)
		query += " AND t.date >= $" + strconv.Itoa(len(args))
	}
	if end != nil {
		args = append(args, *end)
		query += " AND t.date <= $" + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	query += " ORDER BY t.date DESC, t.created_at DESC LIMIT $" + strconv.Itoa(len(args))

	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []TransactionWithEntries{}, nil
	}

	txIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		txIDs = append(txIDs, row.ID)
	}

	entryRows := []EntryRow{}
	queryEntries, argsEntries, err := sqlx.In(`
		SELECT `+entryColumns+`
		FROM entries e
		WHERE e.transaction_id IN (?)
	`, txIDs)
	if err != nil {
		return nil, err
	}
	queryEntries = s.db.Rebind(queryEntries)
	if err := s.db.SelectContext(ctx, &entryRows, queryEntries, argsEntries...); err != nil {
		return nil, err
	}

	entriesByTx := make(map[string][]models.Entry, len(rows))
	for _, row := range entryRows {
		entriesByTx[row.TransactionID] = append(entriesByTx[row.TransactionID], entryFromRow(row))
	}

	out := make([]TransactionWithEntries, 0, len(rows))
	for _, row := range rows {
		out = append(out, TransactionWithEntries{
			Transaction: transactionFromRow(row),
			Entries:     entriesByTx[row.ID],
		})
	}
	return out, nil
}

func (s *TransactionStore) CreateFromDraft(ctx context.Context, draft models.DraftTransaction, receiptID string) (models.Transaction, []models.Entry, error) {
	if err := validateBalanced(draft.Entries); err != nil {
		return models.Transaction{}, nil, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Transaction{}, nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := s.validateEntriesForEntity(ctx, tx, draft.EntityID, draft.Entries); err != nil {
		return models.Transaction{}, nil, err
	}
	if err := s.lockReceiptForAttach(ctx, tx, receiptID, draft.EntityID); err != nil {
		return models.Transaction{}, nil, err
	}

	var trID string
	err = tx.GetContext(ctx, &trID, `
		INSERT INTO transactions (entity_id, date, memo, receipt_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, draft.EntityID, draft.Date, nullString(draft.Memo), receiptID)
	if err != nil {
		return models.Transaction{}, nil, err
	}

	entries := make([]models.Entry, 0, len(draft.Entries))
	for _, entry := range draft.Entries {
		var entryID string
		err = tx.GetContext(ctx, &entryID, `
			INSERT INTO entries (transaction_id, account_id, debit_cents, credit_cents)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, trID, entry.AccountID, entry.DebitCents, entry.CreditCents)
		if err != nil {
			return models.Transaction{}, nil, err
		}
		entries = append(entries, models.Entry{
			ID:            entryID,
			TransactionID: trID,
			AccountID:     entry.AccountID,
			DebitCents:    entry.DebitCents,
			CreditCents:   entry.CreditCents,
		})
	}

	if err := attachReceipt(ctx, tx, receiptID); err != nil {
		return models.Transaction{}, nil, err
	}

	if err := tx.Commit(); err != nil {
		return models.Transaction{}, nil, err
	}

	tr, err := s.GetByID(ctx, trID)
	if err != nil {
		return models.Transaction{}, nil, err
	}
	return tr, entries, nil
}

func (s *TransactionStore) validateEntriesForEntity(ctx context.Context, tx *sqlx.Tx, entityID string, entries []models.DraftEntry) error {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, ok := seen[entry.AccountID]; ok {
			continue
		}
		seen[entry.AccountID] = struct{}{}

		var accountEntityID string
		err := tx.GetContext(ctx, &accountEntityID, `
			SELECT a.entity_id
			FROM accounts a
			WHERE a.id = $1
		`, entry.AccountID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAccountEntityMismatch
			}
			return err
		}
		if accountEntityID != entityID {
			return ErrAccountEntityMismatch
		}
	}
	return nil
}

func (s *TransactionStore) lockReceiptForAttach(ctx context.Context, tx *sqlx.Tx, receiptID, entityID string) error {
	var row struct {
		EntityID   string       `db:"entity_id"`
		AttachedAt sql.NullTime `db:"attached_at"`
	}
	err := tx.GetContext(ctx, &row, `
		SELECT r.entity_id, r.attached_at
		FROM receipts r
		WHERE r.id = $1
		FOR UPDATE
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if row.EntityID != entityID {
		return ErrReceiptEntityMismatch
	}
	if row.AttachedAt.Valid {
		return ErrReceiptAlreadyAttached
	}
	return nil
}

func attachReceipt(ctx context.Context, tx *sqlx.Tx, receiptID string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE receipts
		SET status = 'posted', attached_at = now()
		WHERE id = $1
		  AND attached_at IS NULL
	`, receiptID)
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

func validateBalanced(entries []models.DraftEntry) error {
	var debit, credit int64
	for _, e := range entries {
		debit += e.DebitCents
		credit += e.CreditCents
	}
	if debit != credit {
		return errors.New("entries not balanced")
	}
	if debit == 0 && credit == 0 {
		return errors.New("entries empty")
	}
	return nil
}

func transactionFromRow(row TransactionRow) models.Transaction {
	tr := models.Transaction{
		ID:        row.ID,
		EntityID:  row.EntityID,
		Date:      row.Date,
		CreatedAt: row.CreatedAt,
	}
	if row.Memo.Valid {
		tr.Memo = row.Memo.String
	}
	return tr
}

func entryFromRow(row EntryRow) models.Entry {
	return models.Entry{
		ID:            row.ID,
		TransactionID: row.TransactionID,
		AccountID:     row.AccountID,
		DebitCents:    row.DebitCents,
		CreditCents:   row.CreditCents,
	}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
