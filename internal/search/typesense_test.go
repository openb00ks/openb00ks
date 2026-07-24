package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoopProviderReturnsEmptyResults(t *testing.T) {
	t.Parallel()

	provider := NoopProvider{}
	matches, err := provider.SearchTransactions(context.Background(), TransactionQuery{Query: "internet"})
	if err != nil {
		t.Fatalf("SearchTransactions() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected empty no-op matches, got %d", len(matches))
	}
	docs, err := provider.SearchDocuments(context.Background(), DocumentQuery{Query: "office"})
	if err != nil {
		t.Fatalf("SearchDocuments() error = %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected empty no-op document matches, got %d", len(docs))
	}
	if err := provider.UpsertTransaction(context.Background(), TransactionDocument{ID: "tx-1"}); err != nil {
		t.Fatalf("UpsertTransaction() error = %v", err)
	}
	if err := provider.UpsertDocument(context.Background(), SearchDocument{ID: "receipt-1"}); err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}
	if err := provider.DeleteDocument(context.Background(), "receipt-1"); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}
}

func TestTypesenseSearchRequiresTenantAndEntityScope(t *testing.T) {
	t.Parallel()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              "http://typesense.local",
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	if _, err := provider.SearchTransactions(context.Background(), TransactionQuery{TenantID: "tenant-1", Query: "internet"}); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected ErrScopeRequired for missing entity, got %v", err)
	}
	if _, err := provider.SuggestCandidates(context.Background(), CandidateQuery{EntityID: "entity-1", Query: "internet"}); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected ErrScopeRequired for missing tenant, got %v", err)
	}
	if _, err := provider.SearchDocuments(context.Background(), DocumentQuery{TenantID: "tenant-1", Query: "office"}); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected ErrScopeRequired for document search missing entity, got %v", err)
	}
}

func TestTypesenseSearchTransactions(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAPIKey string
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotAPIKey = r.Header.Get("X-TYPESENSE-API-KEY")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hits":[{
				"text_match": 900000,
				"document":{
					"id":"tx-1",
					"tenant_id":"tenant-1",
					"entity_id":"entity-1",
					"transaction_id":"tx-1",
					"date":"2026-01-02",
					"date_unix":1767312000,
					"memo":"Internet service",
					"description":"Comcast internet",
					"account_ids":["acct-internet"],
					"account_names":["Internet"],
					"account_role_tags":["internet"],
					"amount_cents":6500,
					"source":"transaction"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              server.URL,
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	matches, err := provider.SearchTransactions(context.Background(), TransactionQuery{
		TenantID: "tenant-1",
		EntityID: "entity-1",
		Query:    "comcast",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("SearchTransactions() error = %v", err)
	}
	if gotPath != "/collections/testbooks_transactions/documents/search" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotAPIKey != "xyz" {
		t.Fatalf("missing api key")
	}
	if gotBody["q"] != "comcast" || gotBody["query_by"] == "" {
		t.Fatalf("unexpected search body: %#v", gotBody)
	}
	if !strings.Contains(gotBody["filter_by"].(string), "tenant_id") || !strings.Contains(gotBody["filter_by"].(string), "entity_id") {
		t.Fatalf("missing tenant/entity filter: %#v", gotBody["filter_by"])
	}
	if len(matches) != 1 || matches[0].Document.TransactionID != "tx-1" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestTypesenseSearchDocuments(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hits":[{
				"text_match": 800000,
				"document":{
					"id":"receipt_receipt-1",
					"tenant_id":"tenant-1",
					"entity_id":"entity-1",
					"kind":"receipt",
					"object_id":"receipt-1",
					"title":"office.pdf",
					"subtitle":"uploaded",
					"body":"office receipt",
					"status":"uploaded",
					"date":"2026-01-02",
					"date_unix":1767312000,
					"amount_cents":1200,
					"href":"/receipts/receipt-1"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              server.URL,
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	matches, err := provider.SearchDocuments(context.Background(), DocumentQuery{
		TenantID:  "tenant-1",
		EntityID:  "entity-1",
		Query:     "office",
		Kinds:     []string{"receipt", "import"},
		Statuses:  []string{"uploaded"},
		Tags:      []string{"tax"},
		StartDate: "2026-01-01",
		EndDate:   "2026-01-31",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("SearchDocuments() error = %v", err)
	}
	if gotPath != "/collections/testbooks_documents/documents/search" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotBody["q"] != "office" || gotBody["query_by"] == "" {
		t.Fatalf("unexpected search body: %#v", gotBody)
	}
	filter := gotBody["filter_by"].(string)
	if !strings.Contains(filter, "tenant_id") || !strings.Contains(filter, "entity_id") || !strings.Contains(filter, "kind:=") || !strings.Contains(filter, "status:=") || !strings.Contains(filter, "tags:=") || !strings.Contains(filter, "date_unix:>=") || !strings.Contains(filter, "date_unix:<=") {
		t.Fatalf("missing scoped filter: %#v", filter)
	}
	if len(matches) != 1 || matches[0].Document.ObjectID != "receipt-1" || matches[0].Document.Kind != "receipt" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestTypesenseUpsertTransaction(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotDoc TransactionDocument
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewDecoder(r.Body).Decode(&gotDoc); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              server.URL,
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	err = provider.UpsertTransaction(context.Background(), TransactionDocument{
		TransactionID: "tx-1",
		TenantID:      "tenant-1",
		EntityID:      "entity-1",
		Memo:          "Internet service",
	})
	if err != nil {
		t.Fatalf("UpsertTransaction() error = %v", err)
	}
	if gotPath != "/collections/testbooks_transactions/documents?action=upsert" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotDoc.ID != "tx-1" || gotDoc.Source != "transaction" {
		t.Fatalf("unexpected doc defaults: %#v", gotDoc)
	}
}

func TestTypesenseUpsertDocument(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotDoc SearchDocument
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewDecoder(r.Body).Decode(&gotDoc); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              server.URL,
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	err = provider.UpsertDocument(context.Background(), SearchDocument{
		TenantID: "tenant-1",
		EntityID: "entity-1",
		Kind:     "receipt",
		ObjectID: "receipt-1",
		Title:    "office.pdf",
	})
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}
	if gotPath != "/collections/testbooks_documents/documents?action=upsert" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotDoc.ID != "receipt_receipt-1" || gotDoc.Kind != "receipt" || gotDoc.ObjectID != "receipt-1" {
		t.Fatalf("unexpected doc defaults: %#v", gotDoc)
	}
}

func TestTypesenseEnsureDocumentCollectionPatchesMissingFields(t *testing.T) {
	t.Parallel()

	var patched []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections":
			http.Error(w, `{"message":"collection already exists"}`, http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/collections/testbooks_documents":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fields":[
				{"name":"tenant_id"},
				{"name":"entity_id"},
				{"name":"kind"},
				{"name":"object_id"},
				{"name":"title"},
				{"name":"subtitle"},
				{"name":"body"},
				{"name":"status"},
				{"name":"date"},
				{"name":"date_unix"},
				{"name":"amount_cents"},
				{"name":"href"}
			]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/collections/testbooks_documents":
			var body struct {
				Fields []map[string]interface{} `json:"fields"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			patched = body.Fields
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              server.URL,
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	if err := provider.EnsureDocumentCollection(context.Background()); err != nil {
		t.Fatalf("EnsureDocumentCollection() error = %v", err)
	}
	names := map[string]bool{}
	for _, field := range patched {
		if name, _ := field["name"].(string); name != "" {
			names[name] = true
		}
	}
	for _, want := range []string{"account_id", "account_name", "tags"} {
		if !names[want] {
			t.Fatalf("missing patched field %q in %#v", want, patched)
		}
	}
}

func TestTypesenseUpsertRequiresScope(t *testing.T) {
	t.Parallel()

	provider, err := NewTypesenseProvider(TypesenseConfig{
		URL:              "http://typesense.local",
		APIKey:           "xyz",
		CollectionPrefix: "testbooks",
	})
	if err != nil {
		t.Fatalf("NewTypesenseProvider() error = %v", err)
	}
	if err := provider.UpsertTransaction(context.Background(), TransactionDocument{TransactionID: "tx-1", EntityID: "entity-1"}); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected ErrScopeRequired for missing tenant, got %v", err)
	}
	if err := provider.UpsertDocument(context.Background(), SearchDocument{Kind: "receipt", ObjectID: "receipt-1", EntityID: "entity-1"}); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected ErrScopeRequired for missing tenant, got %v", err)
	}
}

func TestBestCandidate(t *testing.T) {
	t.Parallel()

	candidate, ok := BestCandidate([]Candidate{
		{AccountID: "low", Score: 0.5},
		{AccountID: "high", Score: 0.9},
	}, 0.8)
	if !ok || candidate.AccountID != "high" {
		t.Fatalf("expected high scoring candidate, got %#v ok=%v", candidate, ok)
	}
	if _, ok := BestCandidate([]Candidate{{AccountID: "low", Score: 0.5}}, 0.8); ok {
		t.Fatalf("expected low scoring candidate to be rejected")
	}
}
