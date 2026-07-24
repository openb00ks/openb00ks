package db

import (
	"context"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/openb00ks/openb00ks/internal/search"
)

type SearchSource struct {
	db       *DB
	accounts *AccountStore
}

func NewSearchSource(dbConn *DB, accounts *AccountStore) *SearchSource {
	return &SearchSource{db: dbConn, accounts: accounts}
}

func (s *SearchSource) ListEntities(ctx context.Context, tenantID, entityID string) ([]search.EntityData, error) {
	rows := []EntityRow{}
	query := `
		SELECT ` + entityColumns + `
		FROM entities e
		WHERE 1 = 1
	`
	args := []interface{}{}
	if tenantID != "" {
		args = append(args, tenantID)
		query += " AND e.tenant_id = $" + strconv.Itoa(len(args))
	}
	if entityID != "" {
		args = append(args, entityID)
		query += " AND e.id = $" + strconv.Itoa(len(args))
	}
	query += " ORDER BY e.created_at ASC"
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]search.EntityData, 0, len(rows))
	for _, row := range rows {
		out = append(out, search.EntityData{ID: row.ID, TenantID: row.TenantID})
	}
	return out, nil
}

func (s *SearchSource) ListTransactions(ctx context.Context, entityID string) ([]search.TransactionData, error) {
	rows := []TransactionRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+transactionColumns+`
		FROM transactions t
		WHERE t.entity_id = $1
		ORDER BY t.date ASC, t.created_at ASC
	`, entityID); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []search.TransactionData{}, nil
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
	entriesByTx := make(map[string][]search.EntryData, len(rows))
	for _, row := range entryRows {
		entriesByTx[row.TransactionID] = append(entriesByTx[row.TransactionID], search.EntryData{
			AccountID:   row.AccountID,
			DebitCents:  row.DebitCents,
			CreditCents: row.CreditCents,
		})
	}
	out := make([]search.TransactionData, 0, len(rows))
	for _, row := range rows {
		tr := transactionFromRow(row)
		out = append(out, search.TransactionData{
			ID:        tr.ID,
			EntityID:  tr.EntityID,
			Date:      tr.Date,
			Memo:      tr.Memo,
			CreatedAt: tr.CreatedAt,
			Entries:   entriesByTx[tr.ID],
		})
	}
	return out, nil
}

func (s *SearchSource) ListAccounts(ctx context.Context, entityID string) ([]search.AccountData, error) {
	if s.accounts == nil {
		return []search.AccountData{}, nil
	}
	accounts, err := s.accounts.ListForEntity(ctx, entityID, 1000)
	if err != nil {
		return nil, err
	}
	out := make([]search.AccountData, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, search.AccountData{
			ID:        account.ID,
			EntityID:  account.EntityID,
			Name:      account.Name,
			Type:      account.Type,
			RoleTags:  account.RoleTags,
			CreatedAt: account.CreatedAt,
		})
	}
	return out, nil
}

func (s *SearchSource) ListVendors(ctx context.Context, entityID string) ([]search.VendorData, error) {
	vendors, err := NewVendorStore(s.db).ListForEntity(ctx, entityID, 5000)
	if err != nil {
		return nil, err
	}
	aliasStore := NewVendorAliasStore(s.db)
	out := make([]search.VendorData, 0, len(vendors))
	for _, v := range vendors {
		aliases, aerr := aliasStore.ListNormalized(ctx, v.ID)
		if aerr != nil {
			return nil, aerr
		}
		lastSeen := int64(0)
		if !v.LastSeen.IsZero() {
			lastSeen = v.LastSeen.Unix()
		}
		out = append(out, search.VendorData{
			ID:               v.ID,
			EntityID:         v.EntityID,
			Name:             v.Name,
			MatchPattern:     v.MatchPattern,
			TaxID:            v.TaxID,
			Website:          v.Website,
			DefaultAccountID: v.DefaultAccountID,
			Aliases:          aliases,
			ReceiptCount:     int32(v.ReceiptCount),
			LastSeenUnix:     lastSeen,
		})
	}
	return out, nil
}

func (s *SearchSource) ListReceipts(ctx context.Context, entityID string) ([]search.ReceiptData, error) {
	rows := []ReceiptRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+receiptColumns+`
		FROM receipts r
		WHERE r.entity_id = $1
		ORDER BY r.uploaded_at ASC
	`, entityID); err != nil {
		return nil, err
	}
	receiptIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		receiptIDs = append(receiptIDs, row.ID)
	}
	tagsByReceipt, err := s.receiptTagNames(ctx, receiptIDs)
	if err != nil {
		return nil, err
	}
	out := make([]search.ReceiptData, 0, len(rows))
	for _, row := range rows {
		receipt := receiptFromRow(row)
		out = append(out, search.ReceiptData{
			ID:           receipt.ID,
			EntityID:     receipt.EntityID,
			Kind:         receipt.Kind,
			Status:       receipt.Status,
			ContentType:  receipt.ContentType,
			OriginalName: receipt.OriginalName,
			TotalCents:   receipt.TotalCents,
			TagNames:     tagsByReceipt[receipt.ID],
			UploadedAt:   receipt.UploadedAt,
		})
	}
	return out, nil
}

func (s *SearchSource) ListStatements(ctx context.Context, entityID string) ([]search.StatementData, error) {
	rows := []AccountStatementRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+accountStatementColumns+accountStatementJoins+`
		WHERE s.entity_id = $1
		`+accountStatementGroupBy+`
		ORDER BY s.period_start ASC, s.created_at ASC
	`, entityID); err != nil {
		return nil, err
	}
	out := make([]search.StatementData, 0, len(rows))
	for _, row := range rows {
		statement := accountStatementFromRow(row)
		out = append(out, search.StatementData{
			ID:                 statement.ID,
			EntityID:           statement.EntityID,
			AccountID:          statement.AccountID,
			AccountName:        statement.AccountName,
			AccountType:        statement.AccountType,
			SourceReceiptName:  statement.SourceReceiptName,
			PeriodStart:        statement.PeriodStart,
			PeriodEnd:          statement.PeriodEnd,
			EndingBalanceCents: statement.EndingBalanceCents,
			Status:             statement.Status,
			Notes:              statement.Notes,
			CreatedAt:          statement.CreatedAt,
			UpdatedAt:          statement.UpdatedAt,
		})
	}
	return out, nil
}

func (s *SearchSource) ListMileage(ctx context.Context, entityID string) ([]search.MileageData, error) {
	rows := []MileageLogRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT `+mileageColumns+`
		FROM mileage_logs m
		WHERE m.entity_id = $1
		ORDER BY m.date ASC, m.created_at ASC
	`, entityID); err != nil {
		return nil, err
	}
	out := make([]search.MileageData, 0, len(rows))
	for _, row := range rows {
		mileage := mileageFromRow(row)
		out = append(out, search.MileageData{
			ID:            mileage.ID,
			EntityID:      mileage.EntityID,
			Date:          mileage.Date,
			DistanceMiles: mileage.DistanceMiles,
			StartLocation: mileage.StartLocation,
			EndLocation:   mileage.EndLocation,
			Purpose:       mileage.Purpose,
			CreatedAt:     mileage.CreatedAt,
			UpdatedAt:     mileage.UpdatedAt,
		})
	}
	return out, nil
}

func (s *SearchSource) receiptTagNames(ctx context.Context, receiptIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(receiptIDs))
	if len(receiptIDs) == 0 {
		return out, nil
	}
	type receiptTagRow struct {
		ReceiptID string `db:"receipt_id"`
		Name      string `db:"name"`
	}
	rows := []receiptTagRow{}
	query, args, err := sqlx.In(`
		SELECT rt.receipt_id, t.name
		FROM receipt_tags rt
		JOIN tags t ON t.id = rt.tag_id
		WHERE rt.receipt_id IN (?)
		ORDER BY t.name
	`, receiptIDs)
	if err != nil {
		return nil, err
	}
	query = s.db.Rebind(query)
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ReceiptID] = append(out[row.ReceiptID], row.Name)
	}
	return out, nil
}
