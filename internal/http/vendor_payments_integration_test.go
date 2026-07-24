//go:build integration

package httpapi

import (
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

func TestVendorPayments1099(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendors")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "1000")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	contractors, err := stores.Accounts.Create(ctx, entityID, "Contractors", "expense", "5000")
	if err != nil {
		t.Fatalf("create contractors: %v", err)
	}

	var vendorID string
	if err := conn.GetContext(ctx, &vendorID,
		`INSERT INTO vendors (entity_id, name, normalized_name, tax_id) VALUES ($1, 'Acme LLC', 'acme llc', '12-3456789') RETURNING id`,
		entityID); err != nil {
		t.Fatalf("insert vendor: %v", err)
	}
	var receiptID string
	if err := conn.GetContext(ctx, &receiptID,
		`INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, resolved_vendor_id) VALUES ($1, 'k', 'image/png', 10, $2) RETURNING id`,
		entityID, vendorID); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	trDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := stores.Transactions.Create(ctx, entityID, trDate, "acme invoice", receiptID, []models.DraftEntry{
		{AccountID: contractors.ID, DebitCents: 70000},
		{AccountID: cash.ID, CreditCents: 70000},
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

	req := httptest.NewRequest(http.MethodGet, "/reports/vendor-payments?entity_id="+entityID+"&start_date=2026-01-01&end_date=2026-12-31", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ThresholdCents int64 `json:"threshold_cents"`
		Rows           []struct {
			VendorName string `json:"vendor_name"`
			TaxID      string `json:"tax_id"`
			TotalCents int64  `json:"total_cents"`
			Needs1099  bool   `json:"needs_1099"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("expected 1 vendor row, got %d: %s", len(resp.Rows), w.Body.String())
	}
	row := resp.Rows[0]
	if row.VendorName != "Acme LLC" || row.TotalCents != 70000 || row.TaxID != "12-3456789" || !row.Needs1099 {
		t.Fatalf("unexpected vendor row: %+v", row)
	}
}
