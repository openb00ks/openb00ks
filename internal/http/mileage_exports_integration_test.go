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

func TestMileageExportAndSummaryEndpoints(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "mileage_logs")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	_, err = stores.Mileage.Create(ctx, models.MileageLog{
		EntityID:      entityID,
		UserID:        userID,
		Date:          time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		DistanceMiles: 12.5,
		StartLocation: "Office",
		EndLocation:   "Client",
		Purpose:       "Meeting",
	})
	if err != nil {
		t.Fatalf("create mileage log: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	query := "entity_id=" + entityID + "&start_date=2026-01-01&end_date=2026-01-31"

	req := httptest.NewRequest(http.MethodGet, "/exports/mileage.csv?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "2026-01-10") || !strings.Contains(w.Body.String(), "Office") {
		t.Fatalf("expected mileage row")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/mileage?"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	rows, _ := summary["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 summary row")
	}
	row := rows[0].(map[string]interface{})
	if missing, _ := row["rate_missing"].(bool); !missing {
		t.Fatalf("expected missing rate")
	}
}
