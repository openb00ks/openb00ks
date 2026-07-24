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

func TestSuggestEndpointReturnsStoredSuggestion(t *testing.T) {
	conn := openSuggestTestDB(t)
	ctx := context.Background()
	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	receiptID := insertSuggestTestReceipt(t, conn, entityID)
	defer deleteSuggestTestReceipt(t, conn, receiptID)

	suggestionStore := db.NewReceiptSuggestionStore(conn)
	seeded, err := suggestionStore.Create(ctx, models.ReceiptSuggestion{
		ReceiptID:  receiptID,
		Provider:   "test",
		Model:      "test-model",
		Status:     models.SuggestionStatusSucceeded,
		RawJSON:    []byte(`{"raw":"ok"}`),
		ParsedJSON: []byte(`{"entity_id":"` + entityID + `","entries":[{"account_id":"acct-1"}],"explanation":"Matched stored result"}`),
		Confidence: 0.91,
		RunVersion: 1,
	})
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}

	before := countReceiptSuggestions(t, conn, receiptID)
	server := newSuggestTestServer(t, conn)
	token := issueSuggestTestToken(t, userID, tenantID)

	w := performSuggestRequest(t, server, token, map[string]string{
		"receipt_id": receiptID,
		"text":       "ignored by read-only endpoint",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	after := countReceiptSuggestions(t, conn, receiptID)
	if before != after {
		t.Fatalf("expected read-only endpoint, count before=%d after=%d", before, after)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["receipt_id"] != receiptID {
		t.Fatalf("expected receipt_id %s, got %v", receiptID, resp["receipt_id"])
	}
	if resp["suggestion_id"] != seeded.ID {
		t.Fatalf("expected suggestion_id %s, got %v", seeded.ID, resp["suggestion_id"])
	}
	if resp["entity_id"] != entityID {
		t.Fatalf("expected entity_id %s, got %v", entityID, resp["entity_id"])
	}
	if resp["account_id"] != "acct-1" {
		t.Fatalf("expected account_id acct-1, got %v", resp["account_id"])
	}
	if resp["explanation"] != "Matched stored result" {
		t.Fatalf("expected explanation, got %v", resp["explanation"])
	}
	if resp["source"] != "stored_receipt_suggestion" {
		t.Fatalf("expected stored source, got %v", resp["source"])
	}
}

func TestSuggestEndpointRejectsUnauthorizedReceiptAccess(t *testing.T) {
	conn := openSuggestTestDB(t)
	_, _, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	otherTenantID, otherUserID, _, otherCleanup := setupTenantUserEntity(t, conn)
	defer otherCleanup()

	receiptID := insertSuggestTestReceipt(t, conn, entityID)
	defer deleteSuggestTestReceipt(t, conn, receiptID)

	server := newSuggestTestServer(t, conn)
	token := issueSuggestTestToken(t, otherUserID, otherTenantID)

	w := performSuggestRequest(t, server, token, map[string]string{
		"receipt_id": receiptID,
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSuggestEndpointRequiresReceiptID(t *testing.T) {
	conn := openSuggestTestDB(t)
	tenantID, userID, _, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	server := newSuggestTestServer(t, conn)
	token := issueSuggestTestToken(t, userID, tenantID)

	w := performSuggestRequest(t, server, token, map[string]string{
		"text": "missing receipt id",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "MISSING_FIELDS" {
		t.Fatalf("expected MISSING_FIELDS, got %q", resp["error"])
	}
}

func TestSuggestEndpointReturnsNoSuggestionWithoutRows(t *testing.T) {
	conn := openSuggestTestDB(t)
	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	receiptID := insertSuggestTestReceipt(t, conn, entityID)
	defer deleteSuggestTestReceipt(t, conn, receiptID)

	server := newSuggestTestServer(t, conn)
	token := issueSuggestTestToken(t, userID, tenantID)

	w := performSuggestRequest(t, server, token, map[string]string{
		"receipt_id": receiptID,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "NO_SUGGESTION" {
		t.Fatalf("expected NO_SUGGESTION, got %q", resp["error"])
	}
}

func TestSuggestEndpointFallsBackToVendorRule(t *testing.T) {
	conn := openSuggestTestDB(t)
	ctx := context.Background()
	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	receiptID := insertSuggestTestReceipt(t, conn, entityID)
	defer deleteSuggestTestReceipt(t, conn, receiptID)

	account, err := stores.Accounts.Create(ctx, entityID, "Office Supplies", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	rule, err := stores.VendorRules.Create(ctx, models.VendorRule{
		EntityID:  entityID,
		MatchType: "contains",
		Pattern:   "suggest-",
		AccountID: account.ID,
	})
	if err != nil {
		t.Fatalf("create vendor rule: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM vendor_rules WHERE id = $1`, rule.ID)
	}()

	before := countReceiptSuggestions(t, conn, receiptID)
	server := newSuggestTestServer(t, conn)
	token := issueSuggestTestToken(t, userID, tenantID)

	w := performSuggestRequest(t, server, token, map[string]string{
		"receipt_id": receiptID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	after := countReceiptSuggestions(t, conn, receiptID)
	if before != after {
		t.Fatalf("expected read-only fallback, count before=%d after=%d", before, after)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["source"] != "vendor_rule_fallback" {
		t.Fatalf("expected vendor_rule_fallback, got %v", resp["source"])
	}
	if resp["entity_id"] != entityID {
		t.Fatalf("expected entity_id %s, got %v", entityID, resp["entity_id"])
	}
	if resp["account_id"] != account.ID {
		t.Fatalf("expected account_id %s, got %v", account.ID, resp["account_id"])
	}
	rawPayload, ok := resp["raw_payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected raw_payload object, got %T", resp["raw_payload"])
	}
	if rawPayload["rule_id"] != rule.ID {
		t.Fatalf("expected rule_id %s, got %v", rule.ID, rawPayload["rule_id"])
	}
}

func openSuggestTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipt_suggestions")
	return conn
}

func newSuggestTestServer(t *testing.T, conn *db.DB) *Server {
	t.Helper()
	objects := storage.NewLocalStore(os.TempDir(), "")
	hc := NewHandlerContext(conn, mustSuggestTestTokenService(t), time.Hour, time.Hour, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), nil)
	gin.SetMode(gin.TestMode)
	return NewServer(hc)
}

func mustSuggestTestTokenService(t *testing.T) *auth.TokenService {
	t.Helper()
	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	return tokens
}

func issueSuggestTestToken(t *testing.T, userID, tenantID string) string {
	t.Helper()
	token, err := mustSuggestTestTokenService(t).Issue(userID, tenantID, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return token
}

func insertSuggestTestReceipt(t *testing.T, conn *db.DB, entityID string) string {
	t.Helper()
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()
	var receiptID string
	if err := conn.GetContext(ctx, &receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "suggest-"+suffix, "suggest-"+suffix+".png"); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}
	return receiptID
}

func deleteSuggestTestReceipt(t *testing.T, conn *db.DB, receiptID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `DELETE FROM receipt_suggestions WHERE receipt_id = $1`, receiptID); err != nil {
		t.Fatalf("delete suggestions: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM receipts WHERE id = $1`, receiptID); err != nil {
		t.Fatalf("delete receipt: %v", err)
	}
}

func countReceiptSuggestions(t *testing.T, conn *db.DB, receiptID string) int {
	t.Helper()
	ctx := context.Background()
	var count int
	if err := conn.GetContext(ctx, &count, `SELECT COUNT(*) FROM receipt_suggestions WHERE receipt_id = $1`, receiptID); err != nil {
		t.Fatalf("count suggestions: %v", err)
	}
	return count
}

func performSuggestRequest(t *testing.T, server *Server, token string, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/suggest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	return w
}
