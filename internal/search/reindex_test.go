package search

import (
	"context"
	"testing"
	"time"
)

type fakeReindexSource struct {
	entities     []EntityData
	transactions map[string][]TransactionData
	accounts     map[string][]AccountData
	receipts     map[string][]ReceiptData
	statements   map[string][]StatementData
	mileage      map[string][]MileageData
	vendors      map[string][]VendorData
}

func (f fakeReindexSource) ListEntities(ctx context.Context, tenantID, entityID string) ([]EntityData, error) {
	out := []EntityData{}
	for _, entity := range f.entities {
		if tenantID != "" && entity.TenantID != tenantID {
			continue
		}
		if entityID != "" && entity.ID != entityID {
			continue
		}
		out = append(out, entity)
	}
	return out, nil
}

func (f fakeReindexSource) ListTransactions(ctx context.Context, entityID string) ([]TransactionData, error) {
	return f.transactions[entityID], nil
}

func (f fakeReindexSource) ListAccounts(ctx context.Context, entityID string) ([]AccountData, error) {
	return f.accounts[entityID], nil
}

func (f fakeReindexSource) ListReceipts(ctx context.Context, entityID string) ([]ReceiptData, error) {
	return f.receipts[entityID], nil
}

func (f fakeReindexSource) ListStatements(ctx context.Context, entityID string) ([]StatementData, error) {
	return f.statements[entityID], nil
}

func (f fakeReindexSource) ListMileage(ctx context.Context, entityID string) ([]MileageData, error) {
	return f.mileage[entityID], nil
}

func (f fakeReindexSource) ListVendors(ctx context.Context, entityID string) ([]VendorData, error) {
	return f.vendors[entityID], nil
}

type fakeReindexProvider struct {
	docs       []TransactionDocument
	searchDocs []SearchDocument
	vendorDocs []VendorDocument
}

func (f *fakeReindexProvider) SearchTransactions(ctx context.Context, query TransactionQuery) ([]TransactionMatch, error) {
	return nil, nil
}

func (f *fakeReindexProvider) SearchDocuments(ctx context.Context, query DocumentQuery) ([]DocumentMatch, error) {
	return nil, nil
}

func (f *fakeReindexProvider) SuggestCandidates(ctx context.Context, query CandidateQuery) ([]Candidate, error) {
	return nil, nil
}

func (f *fakeReindexProvider) UpsertTransaction(ctx context.Context, doc TransactionDocument) error {
	f.docs = append(f.docs, doc)
	return nil
}

func (f *fakeReindexProvider) UpsertDocument(ctx context.Context, doc SearchDocument) error {
	f.searchDocs = append(f.searchDocs, doc)
	return nil
}

func (f *fakeReindexProvider) SearchVendors(ctx context.Context, query VendorQuery) ([]VendorMatch, error) {
	return nil, nil
}

func (f *fakeReindexProvider) UpsertVendor(ctx context.Context, doc VendorDocument) error {
	f.vendorDocs = append(f.vendorDocs, doc)
	return nil
}

func (f *fakeReindexProvider) DeleteDocument(ctx context.Context, id string) error {
	return nil
}

func (f *fakeReindexProvider) DeleteVendor(ctx context.Context, id string) error {
	return nil
}

func TestReindexTransactionsScopesEntitiesAndBuildsDocuments(t *testing.T) {
	t.Parallel()

	provider := &fakeReindexProvider{}
	source := fakeReindexSource{
		entities: []EntityData{
			{ID: "entity-1", TenantID: "tenant-1"},
			{ID: "entity-2", TenantID: "tenant-2"},
		},
		transactions: map[string][]TransactionData{
			"entity-1": {
				{
					ID:       "tx-1",
					EntityID: "entity-1",
					Date:     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					Memo:     "Internet service",
					Entries: []EntryData{
						{AccountID: "acct-internet", DebitCents: 6500},
						{AccountID: "acct-cash", CreditCents: 6500},
					},
				},
			},
			"entity-2": {
				{ID: "tx-2", EntityID: "entity-2"},
			},
		},
		accounts: map[string][]AccountData{
			"entity-1": {
				{ID: "acct-internet", EntityID: "entity-1", Name: "Internet", Type: "expense", RoleTags: []string{"internet"}},
				{ID: "acct-cash", EntityID: "entity-1", Name: "Cash", Type: "asset"},
			},
		},
	}

	result, err := (Reindexer{Provider: provider, Source: source}).ReindexTransactions(context.Background(), ReindexOptions{
		TenantID: "tenant-1",
		EntityID: "entity-1",
	})
	if err != nil {
		t.Fatalf("ReindexTransactions() error = %v", err)
	}
	if result.EntityCount != 1 || result.TransactionCount != 1 || result.IndexedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(provider.docs) != 1 {
		t.Fatalf("expected one indexed doc, got %d", len(provider.docs))
	}
	doc := provider.docs[0]
	if doc.TenantID != "tenant-1" || doc.EntityID != "entity-1" || doc.TransactionID != "tx-1" {
		t.Fatalf("unexpected doc scope: %#v", doc)
	}
	if doc.AccountNames[0] != "Internet" || doc.AccountRoleTags[0] != "internet" || doc.AmountCents != 6500 {
		t.Fatalf("unexpected doc content: %#v", doc)
	}
}

func TestReindexDocumentsIndexesTransactionsAndReceipts(t *testing.T) {
	t.Parallel()

	provider := &fakeReindexProvider{}
	source := fakeReindexSource{
		entities: []EntityData{{ID: "entity-1", TenantID: "tenant-1"}},
		transactions: map[string][]TransactionData{
			"entity-1": {
				{
					ID:       "tx-1",
					EntityID: "entity-1",
					Date:     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					Memo:     "Comcast internet",
					Entries:  []EntryData{{AccountID: "acct-internet", DebitCents: 6500}},
				},
			},
		},
		accounts: map[string][]AccountData{
			"entity-1": {{ID: "acct-internet", EntityID: "entity-1", Name: "Internet", Type: "expense", RoleTags: []string{"internet"}}},
		},
		receipts: map[string][]ReceiptData{
			"entity-1": {
				{
					ID:           "receipt-1",
					EntityID:     "entity-1",
					Kind:         "receipt",
					Status:       "uploaded",
					OriginalName: "office.pdf",
					UploadedAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:           "import-1",
					EntityID:     "entity-1",
					Kind:         "import",
					Status:       "ready_for_review",
					OriginalName: "bank.csv",
					UploadedAt:   time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
				},
			},
		},
		statements: map[string][]StatementData{
			"entity-1": {
				{
					ID:                 "stmt-1",
					EntityID:           "entity-1",
					AccountID:          "acct-internet",
					AccountName:        "Internet",
					AccountType:        "expense",
					PeriodStart:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					PeriodEnd:          time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
					EndingBalanceCents: 6500,
					Status:             "reconciled",
				},
			},
		},
		mileage: map[string][]MileageData{
			"entity-1": {
				{
					ID:            "mile-1",
					EntityID:      "entity-1",
					Date:          time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
					DistanceMiles: 12.5,
					StartLocation: "Office",
					EndLocation:   "Client",
					Purpose:       "Client meeting",
				},
			},
		},
	}

	result, err := (Reindexer{Provider: provider, Source: source}).ReindexDocuments(context.Background(), ReindexOptions{
		TenantID: "tenant-1",
		EntityID: "entity-1",
	})
	if err != nil {
		t.Fatalf("ReindexDocuments() error = %v", err)
	}
	if result.DocumentCount != 6 || result.AccountCount != 1 || result.ReceiptCount != 2 || result.StatementCount != 1 || result.MileageCount != 1 || result.IndexedCount != 6 || result.FailedCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	kinds := map[string]bool{}
	for _, doc := range provider.searchDocs {
		kinds[doc.Kind] = true
		if doc.TenantID != "tenant-1" || doc.EntityID != "entity-1" {
			t.Fatalf("unexpected doc scope: %#v", doc)
		}
	}
	for _, want := range []string{"account", "transaction", "receipt", "import", "statement", "mileage"} {
		if !kinds[want] {
			t.Fatalf("missing kind %q in docs %#v", want, provider.searchDocs)
		}
	}
}
