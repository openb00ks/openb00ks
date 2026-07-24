//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestReportAndExportEndpoints(t *testing.T) {
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
		t.Fatalf("create cash account: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "")
	if err != nil {
		t.Fatalf("create revenue account: %v", err)
	}
	trDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if _, _, err := stores.Transactions.Create(ctx, entityID, trDate, "sale", "", []models.DraftEntry{
		{AccountID: cash.ID, DebitCents: 10000},
		{AccountID: revenue.ID, CreditCents: 10000},
	}); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	query := "entity_id=" + entityID + "&start_date=2026-01-01&end_date=2026-01-31"

	req := httptest.NewRequest(http.MethodGet, "/reports/general-ledger?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var ledger map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &ledger); err != nil {
		t.Fatalf("unmarshal ledger: %v", err)
	}
	if rows, _ := ledger["rows"].([]interface{}); len(rows) != 2 {
		t.Fatalf("expected 2 ledger rows")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/profit-loss?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pnl map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &pnl); err != nil {
		t.Fatalf("unmarshal pnl: %v", err)
	}
	if netIncome, _ := pnl["net_income_cents"].(float64); netIncome != 10000 {
		t.Fatalf("expected net income 10000")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/balance-sheet?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var bs map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &bs); err != nil {
		t.Fatalf("unmarshal balance sheet: %v", err)
	}
	if totalAssets, _ := bs["total_assets_cents"].(float64); totalAssets != 10000 {
		t.Fatalf("expected total assets 10000")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/trial-balance?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var tb map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &tb); err != nil {
		t.Fatalf("unmarshal trial balance: %v", err)
	}
	if debit, _ := tb["total_debit_cents"].(float64); debit != 10000 {
		t.Fatalf("expected trial-balance total debit 10000, got %v", tb["total_debit_cents"])
	}
	if credit, _ := tb["total_credit_cents"].(float64); credit != 10000 {
		t.Fatalf("expected trial-balance total credit 10000, got %v", tb["total_credit_cents"])
	}
	if balanced, _ := tb["balanced"].(bool); !balanced {
		t.Fatalf("expected trial balance to be balanced")
	}

	req = httptest.NewRequest(http.MethodGet, "/exports/transactions.csv?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "transaction_id") {
		t.Fatalf("expected csv header")
	}
}
