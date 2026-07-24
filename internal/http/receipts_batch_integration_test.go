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

// TestReceiptSuggestionsBatch_ReadsBodyAfterRoleMiddleware is the regression for the batch-suggestions
// 400: the role middleware (receiptIDsFromBody) reads the request body with ShouldBindBodyWith, which
// consumes + caches it — so the handler must re-read from that cache. A plain ShouldBindJSON hits the
// drained stream (EOF) and 400s EVERY batch call, silently blanking the review queue's AI column and the
// entity dashboard's cost total. This exercises the real route end-to-end and would 400 without the fix.
func TestReceiptSuggestionsBatch_ReadsBodyAfterRoleMiddleware(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipt_suggestions")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	stores := db.NewStores(conn)
	ctx := context.Background()

	receipt, err := stores.Receipts.Create(ctx, entityID, "batch-key", "image/png", "ready_for_review", "receipt", "r.png", 100, 1200)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if _, err := stores.ReceiptSuggestions.Create(ctx, models.ReceiptSuggestion{
		ReceiptID:  receipt.ID,
		Provider:   "openai",
		Model:      "gpt-5-mini",
		Status:     models.SuggestionStatusSucceeded,
		Confidence: 0.9,
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	body, _ := json.Marshal(map[string][]string{"receipt_ids": {receipt.ID}})
	req := httptest.NewRequest(http.MethodPost, "/receipts/suggestions/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("batch suggestions: expected 200 (body re-read from cache), got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Rows []struct {
			ReceiptID string `json:"receipt_id"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	found := false
	for _, row := range resp.Rows {
		if row.ReceiptID == receipt.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the receipt's suggestion in rows, got %s", w.Body.String())
	}
}
