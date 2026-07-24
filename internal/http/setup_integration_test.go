//go:build integration

package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestSetupStatusAndCompletion(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "system_settings")

	var prevComplete bool
	_ = conn.Get(&prevComplete, `SELECT setup_complete FROM system_settings LIMIT 1`)
	defer func() {
		_, _ = conn.Exec(`INSERT INTO system_settings (id, setup_complete, setup_completed_at)
			VALUES (1, $1, NULL)
			ON CONFLICT (id) DO UPDATE
			SET setup_complete = EXCLUDED.setup_complete,
			    setup_completed_at = NULL`, prevComplete)
	}()
	if _, err := conn.Exec(`INSERT INTO system_settings (id, setup_complete, setup_completed_at)
		VALUES (1, false, NULL)
		ON CONFLICT (id) DO UPDATE
		SET setup_complete = EXCLUDED.setup_complete,
		    setup_completed_at = NULL`); err != nil {
		t.Fatalf("reset system settings: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var status SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if !status.Required {
		t.Fatalf("expected required=true before setup")
	}

	payload := map[string]any{
		"tenant_name":    "Test Tenant " + time.Now().UTC().Format("150405"),
		"admin_email":    "admin+" + time.Now().UTC().Format("150405") + "@test.local",
		"admin_password": "secret123",
	}
	body, _ := json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var resp setupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal setup: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/setup/status", nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status.Required {
		t.Fatalf("expected required=false after setup")
	}
	var setupComplete bool
	var completedAt sql.NullTime
	if err := conn.Get(&setupComplete, `SELECT setup_complete FROM system_settings LIMIT 1`); err != nil {
		t.Fatalf("read system settings: %v", err)
	}
	if err := conn.Get(&completedAt, `SELECT setup_completed_at FROM system_settings LIMIT 1`); err != nil {
		t.Fatalf("read system settings time: %v", err)
	}
	if !setupComplete || !completedAt.Valid {
		t.Fatalf("expected setup completion timestamp")
	}

	if resp.AdminUserID != "" {
		_, _ = conn.Exec(`DELETE FROM users WHERE id = $1`, resp.AdminUserID)
	}
	if resp.EntityID != "" {
		_, _ = conn.Exec(`DELETE FROM entities WHERE id = $1`, resp.EntityID)
	}
	if resp.TenantID != "" {
		_, _ = conn.Exec(`DELETE FROM tenants WHERE id = $1`, resp.TenantID)
	}
}
