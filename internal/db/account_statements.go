package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

const accountStatementColumns = `
	s.id,
	s.entity_id,
	s.account_id,
	a.name AS account_name,
	a.type AS account_type,
	s.source_receipt_id,
	r.original_name AS source_receipt_name,
	s.period_start,
	s.period_end,
	s.starting_balance_cents,
	s.ending_balance_cents,
	s.status,
	s.notes,
	s.created_at,
	s.updated_at,
	COALESCE(SUM(CASE WHEN ir.direction = 'inflow' THEN ir.amount_cents ELSE 0 END), 0) AS imported_inflow_cents,
	COALESCE(SUM(CASE WHEN ir.direction <> 'inflow' THEN ir.amount_cents ELSE 0 END), 0) AS imported_outflow_cents,
	COALESCE(SUM(CASE WHEN ir.direction = 'inflow' AND ir.status = 'posted' THEN ir.amount_cents ELSE 0 END), 0) AS posted_inflow_cents,
	COALESCE(SUM(CASE WHEN ir.direction <> 'inflow' AND ir.status = 'posted' THEN ir.amount_cents ELSE 0 END), 0) AS posted_outflow_cents,
	COALESCE(SUM(CASE WHEN ir.id IS NOT NULL AND ir.status <> 'posted' THEN 1 ELSE 0 END), 0) AS unposted_rows
`

const accountStatementJoins = `
	FROM account_statements s
	JOIN accounts a ON a.id = s.account_id
	LEFT JOIN receipts r ON r.id = s.source_receipt_id
	LEFT JOIN import_rows ir ON ir.receipt_id = s.source_receipt_id
	  AND ir.date >= s.period_start
	  AND ir.date <= s.period_end
`

const accountStatementGroupBy = `
	GROUP BY s.id, a.name, a.type, r.original_name
`

type AccountStatementStore struct {
	db *DB
}

type AccountStatementPatch struct {
	AccountID            *string
	SourceReceiptID      *string
	PeriodStart          *time.Time
	PeriodEnd            *time.Time
	StartingBalanceCents *int64
	EndingBalanceCents   *int64
	Status               *string
	Notes                *string
}

func NewAccountStatementStore(db *DB) *AccountStatementStore {
	return &AccountStatementStore{db: db}
}

func (s *AccountStatementStore) List(ctx context.Context, entityID, accountID string, start, end *time.Time, limit int) ([]models.AccountStatement, error) {
	if limit <= 0 {
		limit = 200
	}
	rows := []AccountStatementRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+accountStatementColumns+accountStatementJoins+`
		WHERE s.entity_id = $1
		  AND ($2 = '' OR s.account_id::text = $2)
		  AND ($3::date IS NULL OR s.period_end >= $3)
		  AND ($4::date IS NULL OR s.period_start <= $4)
		`+accountStatementGroupBy+`
		ORDER BY s.period_start DESC, s.created_at DESC
		LIMIT $5
	`, entityID, accountID, start, end, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.AccountStatement, 0, len(rows))
	for _, row := range rows {
		out = append(out, accountStatementFromRow(row))
	}
	return out, nil
}

func (s *AccountStatementStore) GetByID(ctx context.Context, id string) (models.AccountStatement, error) {
	row := AccountStatementRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+accountStatementColumns+accountStatementJoins+`
		WHERE s.id = $1
		`+accountStatementGroupBy+`
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.AccountStatement{}, ErrNotFound
		}
		return models.AccountStatement{}, err
	}
	return accountStatementFromRow(row), nil
}

func (s *AccountStatementStore) GetBySourceReceiptID(ctx context.Context, receiptID string) (models.AccountStatement, error) {
	row := AccountStatementRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+accountStatementColumns+accountStatementJoins+`
		WHERE s.source_receipt_id = $1
		`+accountStatementGroupBy+`
	`, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.AccountStatement{}, ErrNotFound
		}
		return models.AccountStatement{}, err
	}
	return accountStatementFromRow(row), nil
}

func (s *AccountStatementStore) GetEntityID(ctx context.Context, id string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `
		SELECT entity_id
		FROM account_statements
		WHERE id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return entityID, nil
}

func (s *AccountStatementStore) Create(ctx context.Context, statement models.AccountStatement) (models.AccountStatement, error) {
	if statement.Status == "" {
		statement.Status = "draft"
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO account_statements (
			entity_id,
			account_id,
			source_receipt_id,
			period_start,
			period_end,
			starting_balance_cents,
			ending_balance_cents,
			status,
			notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, statement.EntityID, statement.AccountID, nullString(statement.SourceReceiptID), statement.PeriodStart, statement.PeriodEnd, statement.StartingBalanceCents, statement.EndingBalanceCents, statement.Status, nullString(statement.Notes))
	if err != nil {
		return models.AccountStatement{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *AccountStatementStore) Update(ctx context.Context, id string, patch AccountStatementPatch) (models.AccountStatement, error) {
	var accountID sql.NullString
	if patch.AccountID != nil {
		accountID = sql.NullString{String: *patch.AccountID, Valid: true}
	}
	var sourceReceiptID sql.NullString
	if patch.SourceReceiptID != nil {
		sourceReceiptID = sql.NullString{String: *patch.SourceReceiptID, Valid: true}
	}
	var notes sql.NullString
	if patch.Notes != nil {
		notes = sql.NullString{String: *patch.Notes, Valid: true}
	}
	var updatedID string
	err := s.db.GetContext(ctx, &updatedID, `
		UPDATE account_statements
		SET account_id = CASE WHEN $2::text IS NULL THEN account_id ELSE $2::uuid END,
		    source_receipt_id = CASE WHEN $3::text IS NULL THEN source_receipt_id ELSE NULLIF($3, '')::uuid END,
		    period_start = COALESCE($4, period_start),
		    period_end = COALESCE($5, period_end),
		    starting_balance_cents = COALESCE($6, starting_balance_cents),
		    ending_balance_cents = COALESCE($7, ending_balance_cents),
		    status = COALESCE($8, status),
		    notes = CASE WHEN $9::text IS NULL THEN notes ELSE NULLIF($9, '') END,
		    updated_at = now()
		WHERE id = $1
		RETURNING id
	`, id, accountID, sourceReceiptID, patch.PeriodStart, patch.PeriodEnd, patch.StartingBalanceCents, patch.EndingBalanceCents, patch.Status, notes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.AccountStatement{}, ErrNotFound
		}
		return models.AccountStatement{}, err
	}
	return s.GetByID(ctx, updatedID)
}

func (s *AccountStatementStore) Reconcile(ctx context.Context, id string) (models.AccountStatement, error) {
	statement, err := s.GetByID(ctx, id)
	if err != nil {
		return models.AccountStatement{}, err
	}
	status := "reconciled"
	if statement.DifferenceCents != 0 || statement.UnpostedRows > 0 {
		status = "needs_review"
	}
	return s.Update(ctx, id, AccountStatementPatch{Status: &status})
}

func StatementExpectedEndingBalance(startingBalanceCents, importedInflowCents, importedOutflowCents int64) int64 {
	return startingBalanceCents + importedInflowCents - importedOutflowCents
}

func StatementDifferenceCents(endingBalanceCents, expectedEndingBalanceCents int64) int64 {
	return endingBalanceCents - expectedEndingBalanceCents
}

func accountStatementFromRow(row AccountStatementRow) models.AccountStatement {
	importedInflow := nullInt64Value(row.ImportedInflowCents)
	importedOutflow := nullInt64Value(row.ImportedOutflowCents)
	expectedEnding := StatementExpectedEndingBalance(row.StartingBalanceCents, importedInflow, importedOutflow)
	out := models.AccountStatement{
		ID:                         row.ID,
		EntityID:                   row.EntityID,
		AccountID:                  row.AccountID,
		AccountName:                row.AccountName,
		AccountType:                row.AccountType,
		PeriodStart:                row.PeriodStart,
		PeriodEnd:                  row.PeriodEnd,
		StartingBalanceCents:       row.StartingBalanceCents,
		EndingBalanceCents:         row.EndingBalanceCents,
		ImportedInflowCents:        importedInflow,
		ImportedOutflowCents:       importedOutflow,
		PostedInflowCents:          nullInt64Value(row.PostedInflowCents),
		PostedOutflowCents:         nullInt64Value(row.PostedOutflowCents),
		ExpectedEndingBalanceCents: expectedEnding,
		DifferenceCents:            StatementDifferenceCents(row.EndingBalanceCents, expectedEnding),
		UnpostedRows:               int(nullInt64Value(row.UnpostedRows)),
		Status:                     row.Status,
		CreatedAt:                  row.CreatedAt,
		UpdatedAt:                  row.UpdatedAt,
	}
	if row.SourceReceiptID.Valid {
		out.SourceReceiptID = row.SourceReceiptID.String
	}
	if row.SourceReceiptName.Valid {
		out.SourceReceiptName = row.SourceReceiptName.String
	}
	if row.Notes.Valid {
		out.Notes = row.Notes.String
	}
	return out
}

func nullInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}
