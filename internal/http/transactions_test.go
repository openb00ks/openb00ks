package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

type fakeTransactionStore struct {
	created bool
	err     error
}

type fakeEntityStoreTx struct{}

func (f *fakeEntityStoreTx) ListForUser(ctx context.Context, tenantID, userID string, limit int) ([]models.Entity, error) {
	return nil, nil
}

func (f *fakeEntityStoreTx) CreateWithOwner(ctx context.Context, tenantID, userID, name, suggestionContext string) (models.Entity, error) {
	return models.Entity{}, nil
}

func (f *fakeEntityStoreTx) Update(ctx context.Context, tenantID, entityID string, name *string, suggestionContext *string, fiscalYearStartMonth, fiscalYearStartDay *int) (models.Entity, error) {
	return models.Entity{}, nil
}

func (f *fakeEntityStoreTx) Delete(ctx context.Context, tenantID, entityID string) error {
	return nil
}

func (f *fakeEntityStoreTx) GetRole(ctx context.Context, tenantID, userID, entityID string) (models.Role, error) {
	return models.RoleUser, nil
}

func (f *fakeTransactionStore) Create(ctx context.Context, entityID string, date time.Time, memo string, receiptID string, entries []models.DraftEntry) (models.Transaction, []models.Entry, error) {
	if f.err != nil {
		return models.Transaction{}, nil, f.err
	}
	f.created = true
	return models.Transaction{ID: "tr-1", EntityID: entityID, Date: date, Memo: memo, CreatedAt: time.Now()}, []models.Entry{}, nil
}

func (f *fakeTransactionStore) CreateFromDraft(ctx context.Context, draft models.DraftTransaction, receiptID string) (models.Transaction, []models.Entry, error) {
	return models.Transaction{}, nil, nil
}

func (f *fakeTransactionStore) List(ctx context.Context, entityID string, start, end *time.Time, limit int) ([]db.TransactionWithEntries, error) {
	return nil, nil
}

type fakeTransactionSearchProvider struct {
	query         searchpkg.TransactionQuery
	documentQuery searchpkg.DocumentQuery
	docs          []searchpkg.TransactionDocument
	searchDocs    []searchpkg.SearchDocument
	rows          []searchpkg.TransactionMatch
	documentRows  []searchpkg.DocumentMatch
}

func (f *fakeTransactionSearchProvider) SearchTransactions(ctx context.Context, query searchpkg.TransactionQuery) ([]searchpkg.TransactionMatch, error) {
	f.query = query
	return f.rows, nil
}

func (f *fakeTransactionSearchProvider) SearchDocuments(ctx context.Context, query searchpkg.DocumentQuery) ([]searchpkg.DocumentMatch, error) {
	f.documentQuery = query
	return f.documentRows, nil
}

func (f *fakeTransactionSearchProvider) SuggestCandidates(ctx context.Context, query searchpkg.CandidateQuery) ([]searchpkg.Candidate, error) {
	return nil, nil
}

func (f *fakeTransactionSearchProvider) UpsertTransaction(ctx context.Context, doc searchpkg.TransactionDocument) error {
	f.docs = append(f.docs, doc)
	return nil
}

func (f *fakeTransactionSearchProvider) UpsertDocument(ctx context.Context, doc searchpkg.SearchDocument) error {
	f.searchDocs = append(f.searchDocs, doc)
	return nil
}

func (f *fakeTransactionSearchProvider) DeleteDocument(ctx context.Context, id string) error {
	return nil
}

func (f *fakeTransactionSearchProvider) SearchVendors(ctx context.Context, query searchpkg.VendorQuery) ([]searchpkg.VendorMatch, error) {
	return nil, nil
}

func (f *fakeTransactionSearchProvider) UpsertVendor(ctx context.Context, doc searchpkg.VendorDocument) error {
	return nil
}

func (f *fakeTransactionSearchProvider) DeleteVendor(ctx context.Context, id string) error {
	return nil
}

type fakeTransactionSearchSource struct {
	entitiesQuery struct {
		tenantID string
		entityID string
	}
}

func (f *fakeTransactionSearchSource) ListEntities(ctx context.Context, tenantID, entityID string) ([]searchpkg.EntityData, error) {
	f.entitiesQuery.tenantID = tenantID
	f.entitiesQuery.entityID = entityID
	return []searchpkg.EntityData{{ID: entityID, TenantID: tenantID}}, nil
}

func (f *fakeTransactionSearchSource) ListTransactions(ctx context.Context, entityID string) ([]searchpkg.TransactionData, error) {
	return []searchpkg.TransactionData{
		{
			ID:       "tx-1",
			EntityID: entityID,
			Date:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
			Memo:     "Internet service",
			Entries:  []searchpkg.EntryData{{AccountID: "acct-1", DebitCents: 6500}},
		},
	}, nil
}

func (f *fakeTransactionSearchSource) ListAccounts(ctx context.Context, entityID string) ([]searchpkg.AccountData, error) {
	return []searchpkg.AccountData{{ID: "acct-1", EntityID: entityID, Name: "Internet", Type: "expense", RoleTags: []string{"internet"}}}, nil
}

func (f *fakeTransactionSearchSource) ListReceipts(ctx context.Context, entityID string) ([]searchpkg.ReceiptData, error) {
	return []searchpkg.ReceiptData{{ID: "receipt-1", EntityID: entityID, Kind: "receipt", OriginalName: "office.pdf"}}, nil
}

func (f *fakeTransactionSearchSource) ListStatements(ctx context.Context, entityID string) ([]searchpkg.StatementData, error) {
	return []searchpkg.StatementData{}, nil
}

func (f *fakeTransactionSearchSource) ListMileage(ctx context.Context, entityID string) ([]searchpkg.MileageData, error) {
	return []searchpkg.MileageData{}, nil
}

func (f *fakeTransactionSearchSource) ListVendors(ctx context.Context, entityID string) ([]searchpkg.VendorData, error) {
	return []searchpkg.VendorData{}, nil
}

func TestHandleTransactionCreateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeTransactionStore{}
	hc := &HandlerContext{transactions: store, entities: &fakeEntityStoreTx{}}

	r := gin.New()
	r.POST("/transactions", func(c *gin.Context) {
		c.Set(string(userIDKey), "user-1")
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleTransactionCreate(c)
	})

	badBody := map[string]interface{}{"entity_id": "", "date": "", "lines": []interface{}{}}
	payload, _ := json.Marshal(badBody)
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	invalidDate := map[string]interface{}{"entity_id": "e1", "date": "bad", "lines": []map[string]interface{}{{"account_id": "a1", "debit_cents": 10}}}
	payload, _ = json.Marshal(invalidDate)
	req = httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTransactionCreateReceiptAlreadyAttached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeTransactionStore{err: db.ErrReceiptAlreadyAttached}
	hc := &HandlerContext{transactions: store, entities: &fakeEntityStoreTx{}}

	r := gin.New()
	r.POST("/transactions", func(c *gin.Context) {
		c.Set(string(userIDKey), "user-1")
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleTransactionCreate(c)
	})

	body := map[string]interface{}{
		"entity_id":  "e1",
		"date":       "2026-01-05",
		"receipt_id": "r1",
		"lines": []map[string]interface{}{
			{"account_id": "a1", "debit_cents": 10},
			{"account_id": "a2", "credit_cents": 10},
		},
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleTransactionSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searcher := &fakeTransactionSearchProvider{
		rows: []searchpkg.TransactionMatch{
			{
				Document: searchpkg.TransactionDocument{
					TransactionID:   "tx-1",
					EntityID:        "e1",
					Date:            "2026-01-05",
					Memo:            "Internet service",
					AccountIDs:      []string{"acct-1"},
					AccountNames:    []string{"Internet"},
					AccountRoleTags: []string{"internet"},
					AmountCents:     6500,
				},
				Score: 0.95,
			},
		},
	}
	hc := &HandlerContext{search: searcher}

	r := gin.New()
	r.GET("/search/transactions", func(c *gin.Context) {
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleTransactionSearch(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/search/transactions?entity_id=e1&q=internet&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if searcher.query.TenantID != "tenant-1" || searcher.query.EntityID != "e1" || searcher.query.Query != "internet" || searcher.query.Limit != 5 {
		t.Fatalf("unexpected search query: %#v", searcher.query)
	}
	var body struct {
		Rows []struct {
			TransactionID string   `json:"transaction_id"`
			AccountNames  []string `json:"account_names"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Rows) != 1 || body.Rows[0].TransactionID != "tx-1" || body.Rows[0].AccountNames[0] != "Internet" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestHandleUnifiedSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searcher := &fakeTransactionSearchProvider{
		documentRows: []searchpkg.DocumentMatch{
			{
				Document: searchpkg.SearchDocument{
					ID:          "receipt_receipt-1",
					EntityID:    "e1",
					Kind:        "receipt",
					ObjectID:    "receipt-1",
					Title:       "office.pdf",
					Subtitle:    "uploaded",
					Body:        "office receipt",
					Status:      "uploaded",
					Date:        "2026-01-02",
					AmountCents: 1200,
					Href:        "/receipts/receipt-1",
				},
				Score: 0.9,
			},
		},
	}
	hc := &HandlerContext{search: searcher}

	r := gin.New()
	r.GET("/search", func(c *gin.Context) {
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleSearch(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/search?entity_id=e1&q=office&kinds=receipt,import&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if searcher.documentQuery.TenantID != "tenant-1" || searcher.documentQuery.EntityID != "e1" || searcher.documentQuery.Query != "office" || searcher.documentQuery.Limit != 5 {
		t.Fatalf("unexpected document query: %#v", searcher.documentQuery)
	}
	if len(searcher.documentQuery.Kinds) != 2 || searcher.documentQuery.Kinds[0] != "receipt" || searcher.documentQuery.Kinds[1] != "import" {
		t.Fatalf("unexpected kinds: %#v", searcher.documentQuery.Kinds)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"href":"/receipts/receipt-1"`)) {
		t.Fatalf("missing search hit response: %s", w.Body.String())
	}
}

func TestHandleTransactionSearchRequiresQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{search: searchpkg.NoopProvider{}}

	r := gin.New()
	r.GET("/search/transactions", hc.handleTransactionSearch)

	req := httptest.NewRequest(http.MethodGet, "/search/transactions?entity_id=e1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTransactionSearchReindexScopesTenantAndEntity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searcher := &fakeTransactionSearchProvider{}
	source := &fakeTransactionSearchSource{}
	hc := &HandlerContext{search: searcher, searchSource: source}

	r := gin.New()
	r.POST("/search/transactions/reindex", func(c *gin.Context) {
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleTransactionSearchReindex(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/search/transactions/reindex?entity_id=entity-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if source.entitiesQuery.tenantID != "tenant-1" || source.entitiesQuery.entityID != "entity-1" {
		t.Fatalf("unexpected reindex scope: %#v", source.entitiesQuery)
	}
	if len(searcher.docs) != 1 || searcher.docs[0].TenantID != "tenant-1" || searcher.docs[0].EntityID != "entity-1" {
		t.Fatalf("unexpected indexed docs: %#v", searcher.docs)
	}
}

func TestHandleUnifiedSearchReindexScopesTenantAndEntity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searcher := &fakeTransactionSearchProvider{}
	source := &fakeTransactionSearchSource{}
	hc := &HandlerContext{search: searcher, searchSource: source}

	r := gin.New()
	r.POST("/search/reindex", func(c *gin.Context) {
		c.Set(string(tenantIDKey), "tenant-1")
		hc.handleSearchReindex(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/search/reindex?entity_id=entity-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if source.entitiesQuery.tenantID != "tenant-1" || source.entitiesQuery.entityID != "entity-1" {
		t.Fatalf("unexpected reindex scope: %#v", source.entitiesQuery)
	}
	if len(searcher.searchDocs) != 3 {
		t.Fatalf("expected account, transaction, and receipt documents, got %#v", searcher.searchDocs)
	}
	kinds := map[string]bool{}
	for _, doc := range searcher.searchDocs {
		if doc.TenantID != "tenant-1" || doc.EntityID != "entity-1" {
			t.Fatalf("unexpected indexed document scope: %#v", doc)
		}
		kinds[doc.Kind] = true
	}
	if !kinds["account"] || !kinds["transaction"] || !kinds["receipt"] {
		t.Fatalf("missing unified document kinds: %#v", searcher.searchDocs)
	}
}
