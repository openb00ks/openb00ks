//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestTransactionsCreateAndList(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "transactions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "")
	if err != nil {
		t.Fatalf("create revenue: %v", err)
	}
	receipt, err := stores.Receipts.Create(ctx, entityID, "receipt-"+testutil.UniqueSuffix(), "image/png", "uploaded", "receipt", "sale.png", 1, 2500)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	body, _ := json.Marshal(map[string]interface{}{
		"entity_id":  entityID,
		"date":       "2026-01-05",
		"memo":       "sale",
		"receipt_id": receipt.ID,
		"lines": []map[string]interface{}{
			{"account_id": cash.ID, "debit_cents": 2500},
			{"account_id": revenue.ID, "credit_cents": 2500},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	entries, _ := created["entries"].([]interface{})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries")
	}
	attached, err := stores.Receipts.GetByID(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("get attached receipt: %v", err)
	}
	if attached.AttachedAt == nil || attached.Status != "posted" {
		t.Fatalf("expected receipt attached and posted")
	}

	req = httptest.NewRequest(http.MethodGet, "/transactions?entity_id="+entityID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var listed map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	rows, _ := listed["rows"].([]interface{})
	if len(rows) == 0 {
		t.Fatalf("expected rows")
	}

	_, _, _ = stores.Transactions.Create(ctx, entityID, time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "extra", "", []models.DraftEntry{
		{AccountID: cash.ID, DebitCents: 100},
		{AccountID: revenue.ID, CreditCents: 100},
	})
	req = httptest.NewRequest(http.MethodGet, "/transactions?entity_id="+entityID+"&start_date=2026-01-06", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var filtered map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered: %v", err)
	}
	filterRows, _ := filtered["rows"].([]interface{})
	if len(filterRows) == 0 {
		t.Fatalf("expected filtered rows")
	}
}

func TestTransactionsRejectAccountFromAnotherEntity(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "transactions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	var otherEntityID string
	if err := conn.GetContext(ctx, &otherEntityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "other-"+testutil.UniqueSuffix()); err != nil {
		t.Fatalf("create other entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, otherEntityID)
	}()
	foreignAccount, err := stores.Accounts.Create(ctx, otherEntityID, "Foreign Revenue", "income", "")
	if err != nil {
		t.Fatalf("create foreign account: %v", err)
	}

	server, token := newTransactionIntegrationServer(t, conn, userID, tenantID)
	body, _ := json.Marshal(map[string]interface{}{
		"entity_id": entityID,
		"date":      "2026-01-05",
		"lines": []map[string]interface{}{
			{"account_id": cash.ID, "debit_cents": 1000},
			{"account_id": foreignAccount.ID, "credit_cents": 1000},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	assertAPIError(t, w.Body.Bytes(), "INVALID_TRANSACTION")
}

func TestTransactionsRejectReceiptFromAnotherEntity(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "transactions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "")
	if err != nil {
		t.Fatalf("create revenue: %v", err)
	}
	var otherEntityID string
	if err := conn.GetContext(ctx, &otherEntityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "other-"+testutil.UniqueSuffix()); err != nil {
		t.Fatalf("create other entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, otherEntityID)
	}()
	foreignReceipt, err := stores.Receipts.Create(ctx, otherEntityID, "receipt-"+testutil.UniqueSuffix(), "image/png", "uploaded", "receipt", "foreign.png", 1, 1000)
	if err != nil {
		t.Fatalf("create foreign receipt: %v", err)
	}

	server, token := newTransactionIntegrationServer(t, conn, userID, tenantID)
	body, _ := json.Marshal(map[string]interface{}{
		"entity_id":  entityID,
		"date":       "2026-01-05",
		"receipt_id": foreignReceipt.ID,
		"lines": []map[string]interface{}{
			{"account_id": cash.ID, "debit_cents": 1000},
			{"account_id": revenue.ID, "credit_cents": 1000},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	assertAPIError(t, w.Body.Bytes(), "INVALID_TRANSACTION")
}

func TestTransactionsRejectDuplicateReceiptAttachment(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "transactions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "")
	if err != nil {
		t.Fatalf("create revenue: %v", err)
	}
	receipt, err := stores.Receipts.Create(ctx, entityID, "receipt-"+testutil.UniqueSuffix(), "image/png", "uploaded", "receipt", "dup.png", 1, 1000)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}

	server, token := newTransactionIntegrationServer(t, conn, userID, tenantID)
	body, _ := json.Marshal(map[string]interface{}{
		"entity_id":  entityID,
		"date":       "2026-01-05",
		"receipt_id": receipt.ID,
		"lines": []map[string]interface{}{
			{"account_id": cash.ID, "debit_cents": 1000},
			{"account_id": revenue.ID, "credit_cents": 1000},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	assertAPIError(t, w.Body.Bytes(), "RECEIPT_ALREADY_ATTACHED")
}

func TestDraftPostRejectsAlreadyAttachedReceipt(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "draft_transactions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "")
	if err != nil {
		t.Fatalf("create revenue: %v", err)
	}
	receipt, err := stores.Receipts.Create(ctx, entityID, "receipt-"+testutil.UniqueSuffix(), "image/png", "uploaded", "receipt", "draft.png", 1, 1000)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if _, err := stores.Drafts.EnsureForReceipt(ctx, receipt.ID); err != nil {
		t.Fatalf("ensure draft: %v", err)
	}
	updatedDraft, err := stores.Drafts.UpdateDraft(ctx, receipt.ID, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "draft", []models.DraftEntry{
		{AccountID: cash.ID, DebitCents: 1000},
		{AccountID: revenue.ID, CreditCents: 1000},
	})
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updatedDraft.ID == "" {
		t.Fatalf("expected draft id")
	}
	if _, _, err := stores.Transactions.Create(ctx, entityID, time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "posted", receipt.ID, []models.DraftEntry{
		{AccountID: cash.ID, DebitCents: 1000},
		{AccountID: revenue.ID, CreditCents: 1000},
	}); err != nil {
		t.Fatalf("attach receipt via transaction: %v", err)
	}

	server, token := newTransactionIntegrationServer(t, conn, userID, tenantID)
	req := httptest.NewRequest(http.MethodPost, "/receipts/"+receipt.ID+"/post", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	assertAPIError(t, w.Body.Bytes(), "RECEIPT_ALREADY_ATTACHED")
}

func newTransactionIntegrationServer(t *testing.T, conn *db.DB, userID, tenantID string) (*Server, string) {
	t.Helper()
	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	token, err := tokens.Issue(userID, tenantID, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), nil)
	gin.SetMode(gin.TestMode)
	return NewServer(hc), token
}

func assertAPIError(t *testing.T, body []byte, want string) {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if payload["error"] != want {
		t.Fatalf("expected %s, got %q", want, payload["error"])
	}
}
