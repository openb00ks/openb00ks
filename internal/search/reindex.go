package search

import (
	"context"
	"fmt"
	"time"
)

type Reindexer struct {
	Provider Provider
	Source   TransactionSource
}

type TransactionSource interface {
	ListEntities(ctx context.Context, tenantID, entityID string) ([]EntityData, error)
	ListTransactions(ctx context.Context, entityID string) ([]TransactionData, error)
	ListAccounts(ctx context.Context, entityID string) ([]AccountData, error)
	ListReceipts(ctx context.Context, entityID string) ([]ReceiptData, error)
	ListStatements(ctx context.Context, entityID string) ([]StatementData, error)
	ListMileage(ctx context.Context, entityID string) ([]MileageData, error)
	ListVendors(ctx context.Context, entityID string) ([]VendorData, error)
}

type EntityData struct {
	ID       string
	TenantID string
}

type TransactionData struct {
	ID        string
	EntityID  string
	Date      time.Time
	Memo      string
	CreatedAt time.Time
	Entries   []EntryData
}

type ReindexOptions struct {
	TenantID string
	EntityID string
}

type ReindexResult struct {
	EntityCount      int `json:"entity_count"`
	AccountCount     int `json:"account_count"`
	TransactionCount int `json:"transaction_count"`
	ReceiptCount     int `json:"receipt_count"`
	StatementCount   int `json:"statement_count"`
	MileageCount     int `json:"mileage_count"`
	VendorCount      int `json:"vendor_count"`
	DocumentCount    int `json:"document_count"`
	IndexedCount     int `json:"indexed_count"`
	FailedCount      int `json:"failed_count"`
}

func (r Reindexer) ReindexTransactions(ctx context.Context, opts ReindexOptions) (ReindexResult, error) {
	if r.Provider == nil {
		return ReindexResult{}, fmt.Errorf("search provider is required")
	}
	if r.Source == nil {
		return ReindexResult{}, fmt.Errorf("transaction source is required")
	}
	entities, err := r.Source.ListEntities(ctx, opts.TenantID, opts.EntityID)
	if err != nil {
		return ReindexResult{}, err
	}
	result := ReindexResult{EntityCount: len(entities)}
	for _, entity := range entities {
		accounts, err := r.Source.ListAccounts(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		transactions, err := r.Source.ListTransactions(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.TransactionCount += len(transactions)
		for _, tx := range transactions {
			doc := TransactionDocumentFromData(entity.TenantID, tx.ID, tx.EntityID, tx.Date, tx.Memo, tx.Entries, accounts, tx.CreatedAt)
			if err := r.Provider.UpsertTransaction(ctx, doc); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
	}
	return result, nil
}

func (r Reindexer) ReindexDocuments(ctx context.Context, opts ReindexOptions) (ReindexResult, error) {
	if r.Provider == nil {
		return ReindexResult{}, fmt.Errorf("search provider is required")
	}
	if r.Source == nil {
		return ReindexResult{}, fmt.Errorf("search source is required")
	}
	entities, err := r.Source.ListEntities(ctx, opts.TenantID, opts.EntityID)
	if err != nil {
		return ReindexResult{}, err
	}
	result := ReindexResult{EntityCount: len(entities)}
	for _, entity := range entities {
		accounts, err := r.Source.ListAccounts(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.AccountCount += len(accounts)
		for _, account := range accounts {
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromAccount(entity.TenantID, account)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
		transactions, err := r.Source.ListTransactions(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.TransactionCount += len(transactions)
		for _, tx := range transactions {
			txDoc := TransactionDocumentFromData(entity.TenantID, tx.ID, tx.EntityID, tx.Date, tx.Memo, tx.Entries, accounts, tx.CreatedAt)
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromTransaction(txDoc)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
		receipts, err := r.Source.ListReceipts(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.ReceiptCount += len(receipts)
		for _, receipt := range receipts {
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromReceipt(entity.TenantID, receipt)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
		statements, err := r.Source.ListStatements(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.StatementCount += len(statements)
		for _, statement := range statements {
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromStatement(entity.TenantID, statement)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
		mileageRows, err := r.Source.ListMileage(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.MileageCount += len(mileageRows)
		for _, mileage := range mileageRows {
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromMileage(entity.TenantID, mileage)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
		}
		vendors, err := r.Source.ListVendors(ctx, entity.ID)
		if err != nil {
			return result, err
		}
		result.VendorCount += len(vendors)
		for _, vendor := range vendors {
			result.DocumentCount++
			if err := r.Provider.UpsertDocument(ctx, SearchDocumentFromVendor(entity.TenantID, vendor)); err != nil {
				result.FailedCount++
				continue
			}
			result.IndexedCount++
			// Also index into the dedicated _vendors retrieval collection (best-effort; the global-search
			// document above is what the reindex counts reflect).
			_ = r.Provider.UpsertVendor(ctx, VendorDocumentFromData(entity.TenantID, vendor))
		}
	}
	return result, nil
}
