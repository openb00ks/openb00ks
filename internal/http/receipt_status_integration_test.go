//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/queue"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestReceiptStatusEndpoint(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipt_jobs")
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.Get(&tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var userID string
	if err := conn.Get(&userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "rs-"+suffix+"@test.local", "hash"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM users WHERE id = $1`, userID)
	}()

	if _, err := conn.Exec(`INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantID, userID); err != nil {
		t.Fatalf("insert tenant membership: %v", err)
	}
	if _, err := conn.Exec(`UPDATE users SET default_tenant_id = $2 WHERE id = $1`, userID, tenantID); err != nil {
		t.Fatalf("set default tenant: %v", err)
	}

	var entityID string
	if err := conn.Get(&entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM entity_users WHERE entity_id = $1`, entityID)
		_, _ = conn.Exec(`DELETE FROM receipts WHERE entity_id = $1`, entityID)
		_, _ = conn.Exec(`DELETE FROM entities WHERE id = $1`, entityID)
	}()

	if _, err := conn.Exec(`INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityID); err != nil {
		t.Fatalf("insert entity membership: %v", err)
	}

	var receiptID string
	if err := conn.Get(&receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "test-"+suffix, "test-"+suffix+".png"); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	if _, err := conn.Exec(`
		INSERT INTO receipt_jobs (receipt_id, stage, status)
		VALUES ($1, 'ocr', 'queued')
	`, receiptID); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	if _, err := conn.Exec(`
		INSERT INTO processing_errors (entity_id, receipt_id, stage, error)
		VALUES ($1, $2, 'ocr', 'boom')
	`, entityID, receiptID); err != nil {
		t.Fatalf("insert error: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(os.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), queue.NewDBQueue(conn))
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "/receipts/"+receiptID+"/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["receipt_id"].(string) != receiptID {
		t.Fatalf("expected receipt id")
	}
	errs := resp["errors"].([]interface{})
	if len(errs) == 0 {
		t.Fatalf("expected errors list")
	}
}
