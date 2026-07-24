//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestImportWorkflowEndpoints(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipts")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	dir := t.TempDir()
	objects := storage.NewLocalStore(dir, "http://localhost/receipts")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	form := url.Values{}
	form.Set("entity_id", entityID)
	form.Set("text", "date,amount\n2026-01-01,10.00\n")
	form.Set("content_type", "text/csv")
	form.Set("filename", "import.csv")
	form.Set("suggestion_context", "import-context")

	req := httptest.NewRequest(http.MethodPost, "/imports", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	importID, _ := created["id"].(string)
	if importID == "" {
		t.Fatal("missing import id")
	}

	req = httptest.NewRequest(http.MethodGet, "/imports?entity_id="+entityID, nil)
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

	req = httptest.NewRequest(http.MethodGet, "/imports/"+importID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var fetched map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if fetched["suggestion_context"] != "import-context" {
		t.Fatalf("expected suggestion context")
	}

	suggestStore := db.NewReceiptSuggestionStore(conn)
	if _, err := suggestStore.Create(context.Background(), models.ReceiptSuggestion{
		ReceiptID:  importID,
		Provider:   "test",
		Model:      "test-model",
		Status:     "ok",
		PromptJSON: []byte(`{"prompt": "x"}`),
		RawJSON:    []byte(`{"raw": "y"}`),
		ParsedJSON: []byte(`{"parsed": "z"}`),
		RunVersion: 1,
	}); err != nil {
		t.Fatalf("insert suggestion: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/imports/"+importID+"/suggestion", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var suggestResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &suggestResp); err != nil {
		t.Fatalf("unmarshal suggestion: %v", err)
	}
	suggestRows, _ := suggestResp["rows"].([]interface{})
	if len(suggestRows) != 1 {
		t.Fatalf("expected 1 suggestion row")
	}
}
