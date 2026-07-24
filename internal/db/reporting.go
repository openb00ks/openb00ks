package db

import (
	"context"
	"database/sql"
	"time"
)

// GeneralLedgerRow represents a single entry with account + transaction context.
type GeneralLedgerRow struct {
	TransactionID string    `json:"transaction_id"`
	Date          time.Time `json:"date"`
	Memo          string    `json:"memo"`
	AccountID     string    `json:"account_id"`
	AccountName   string    `json:"account_name"`
	AccountType   string    `json:"account_type"`
	DebitCents    int64     `json:"debit_cents"`
	CreditCents   int64     `json:"credit_cents"`
	SourceKind    string    `json:"source_kind"`
	SourceID      string    `json:"source_id"`
	SourceName    string    `json:"source_name"`
	SourceRow     int       `json:"source_row"`
	SourceVendor  string    `json:"source_vendor"`
	SourceHash    string    `json:"source_hash"`
}

type generalLedgerRowDB struct {
	TransactionID string         `db:"transaction_id"`
	Date          time.Time      `db:"date"`
	Memo          string         `db:"memo"`
	AccountID     string         `db:"account_id"`
	AccountName   string         `db:"account_name"`
	AccountType   string         `db:"account_type"`
	DebitCents    int64          `db:"debit_cents"`
	CreditCents   int64          `db:"credit_cents"`
	SourceKind    sql.NullString `db:"source_kind"`
	SourceID      sql.NullString `db:"source_id"`
	SourceName    sql.NullString `db:"source_name"`
	SourceRow     sql.NullInt64  `db:"source_row"`
	SourceVendor  sql.NullString `db:"source_vendor"`
	SourceHash    sql.NullString `db:"source_hash"`
}

// TrialBalanceRow can be used for P&L/Balance Sheet summaries.
type TrialBalanceRow struct {
	AccountID   string `db:"account_id"`
	AccountName string `db:"account_name"`
	AccountType string `db:"account_type"`
	DebitCents  int64  `db:"debit_cents"`
	CreditCents int64  `db:"credit_cents"`
}

type ReportingStore struct {
	db *DB
}

func NewReportingStore(db *DB) *ReportingStore {
	return &ReportingStore{db: db}
}

func (s *ReportingStore) GeneralLedger(ctx context.Context, entityID string, start, end time.Time) ([]GeneralLedgerRow, error) {
	rows := []generalLedgerRowDB{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT t.id AS transaction_id, t.date, COALESCE(t.memo, '') AS memo,
		       a.id AS account_id, a.name AS account_name, a.type AS account_type,
		       e.debit_cents, e.credit_cents,
		       CASE
		         WHEN ir.id IS NOT NULL THEN 'import_row'
		         WHEN tr.id IS NOT NULL THEN 'receipt'
		         ELSE ''
		       END AS source_kind,
		       COALESCE(ir.receipt_id::text, tr.id::text, '') AS source_id,
		       COALESCE(sr.original_name, tr.original_name, '') AS source_name,
		       ir.row_index AS source_row,
		       COALESCE(ir.vendor, '') AS source_vendor,
		       COALESCE(ir.fingerprint, '') AS source_hash
		FROM transactions t
		JOIN entries e ON e.transaction_id = t.id
		JOIN accounts a ON a.id = e.account_id
		LEFT JOIN import_rows ir ON ir.transaction_id = t.id
		LEFT JOIN receipts sr ON sr.id = ir.receipt_id
		LEFT JOIN receipts tr ON tr.id = t.receipt_id
		WHERE t.entity_id = $1
		  AND t.date >= $2 AND t.date <= $3
		ORDER BY t.date, t.id
	`, entityID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]GeneralLedgerRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, generalLedgerFromDB(row))
	}
	return out, nil
}

func (s *ReportingStore) TrialBalance(ctx context.Context, entityID string, start, end time.Time) ([]TrialBalanceRow, error) {
	rows := []TrialBalanceRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT a.id AS account_id, a.name AS account_name, a.type AS account_type,
		       SUM(e.debit_cents) AS debit_cents,
		       SUM(e.credit_cents) AS credit_cents
		FROM transactions t
		JOIN entries e ON e.transaction_id = t.id
		JOIN accounts a ON a.id = e.account_id
		WHERE t.entity_id = $1
		  AND t.date >= $2 AND t.date <= $3
		GROUP BY a.id, a.name, a.type
		ORDER BY a.name
	`, entityID, start, end)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ReportingStore) TransactionsForExport(ctx context.Context, entityID string, start, end time.Time) ([]GeneralLedgerRow, error) {
	return s.GeneralLedger(ctx, entityID, start, end)
}

// VendorPaymentRow is the total expense paid to a first-class vendor in a period — the basis for a
// 1099-NEC candidate list. Payments are attributed via a transaction's source receipt's resolved vendor;
// only the expense side of each entry is summed (the cash/credit side is ignored).
type VendorPaymentRow struct {
	VendorID   string `db:"vendor_id"`
	VendorName string `db:"vendor_name"`
	TaxID      string `db:"tax_id"`
	TotalCents int64  `db:"total_cents"`
}

func (s *ReportingStore) VendorPayments(ctx context.Context, entityID string, start, end time.Time) ([]VendorPaymentRow, error) {
	rows := []VendorPaymentRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT v.id AS vendor_id, v.name AS vendor_name, COALESCE(v.tax_id, '') AS tax_id,
		       SUM(e.debit_cents - e.credit_cents) AS total_cents
		FROM transactions t
		JOIN receipts r ON r.id = t.receipt_id
		JOIN vendors v ON v.id = r.resolved_vendor_id
		JOIN entries e ON e.transaction_id = t.id
		JOIN accounts a ON a.id = e.account_id AND a.type = 'expense'
		WHERE t.entity_id = $1 AND t.date >= $2 AND t.date <= $3
		GROUP BY v.id, v.name, v.tax_id
		HAVING SUM(e.debit_cents - e.credit_cents) > 0
		ORDER BY SUM(e.debit_cents - e.credit_cents) DESC, v.name
	`, entityID, start, end)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AccountBalanceRow is an all-time debit/credit total per account (every account for the entity, including
// those with no postings). The natural balance sign is applied by the caller based on account type.
type AccountBalanceRow struct {
	AccountID   string `db:"account_id"`
	AccountType string `db:"account_type"`
	DebitCents  int64  `db:"debit_cents"`
	CreditCents int64  `db:"credit_cents"`
}

func (s *ReportingStore) AccountBalances(ctx context.Context, entityID string) ([]AccountBalanceRow, error) {
	rows := []AccountBalanceRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT a.id AS account_id, a.type AS account_type,
		       COALESCE(SUM(e.debit_cents), 0) AS debit_cents,
		       COALESCE(SUM(e.credit_cents), 0) AS credit_cents
		FROM accounts a
		LEFT JOIN entries e ON e.account_id = a.id
		WHERE a.entity_id = $1
		GROUP BY a.id, a.type
	`, entityID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AccountLedgerRow is one posted journal line for a single account, newest first.
type AccountLedgerRow struct {
	TransactionID string    `db:"transaction_id"`
	Date          time.Time `db:"date"`
	Memo          string    `db:"memo"`
	DebitCents    int64     `db:"debit_cents"`
	CreditCents   int64     `db:"credit_cents"`
}

func (s *ReportingStore) AccountLedger(ctx context.Context, entityID, accountID string, limit, offset int) ([]AccountLedgerRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows := []AccountLedgerRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT t.id AS transaction_id, t.date, COALESCE(t.memo, '') AS memo,
		       e.debit_cents, e.credit_cents
		FROM entries e
		JOIN transactions t ON t.id = e.transaction_id
		WHERE t.entity_id = $1 AND e.account_id = $2
		ORDER BY t.date DESC, t.id DESC
		LIMIT $3 OFFSET $4
	`, entityID, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func generalLedgerFromDB(row generalLedgerRowDB) GeneralLedgerRow {
	out := GeneralLedgerRow{
		TransactionID: row.TransactionID,
		Date:          row.Date,
		Memo:          row.Memo,
		AccountID:     row.AccountID,
		AccountName:   row.AccountName,
		AccountType:   row.AccountType,
		DebitCents:    row.DebitCents,
		CreditCents:   row.CreditCents,
	}
	if row.SourceKind.Valid {
		out.SourceKind = row.SourceKind.String
	}
	if row.SourceID.Valid {
		out.SourceID = row.SourceID.String
	}
	if row.SourceName.Valid {
		out.SourceName = row.SourceName.String
	}
	if row.SourceRow.Valid {
		out.SourceRow = int(row.SourceRow.Int64)
	}
	if row.SourceVendor.Valid {
		out.SourceVendor = row.SourceVendor.String
	}
	if row.SourceHash.Valid {
		out.SourceHash = row.SourceHash.String
	}
	return out
}
