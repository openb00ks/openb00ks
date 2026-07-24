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

func TestTenantIsolationEntities(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "tenants")

	suffix := testutil.UniqueSuffix()
	var userID string
	if err := conn.Get(&userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "iso-"+suffix+"@test.local", "hash"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM users WHERE id = $1`, userID)
	}()

	var tenantA string
	if err := conn.Get(&tenantA, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-a-"+suffix); err != nil {
		t.Fatalf("insert tenant a: %v", err)
	}
	var tenantB string
	if err := conn.Get(&tenantB, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-b-"+suffix); err != nil {
		t.Fatalf("insert tenant b: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM tenants WHERE id = $1 OR id = $2`, tenantA, tenantB)
	}()

	if _, err := conn.Exec(`INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantA, userID); err != nil {
		t.Fatalf("insert tenant membership: %v", err)
	}
	if _, err := conn.Exec(`UPDATE users SET default_tenant_id = $2 WHERE id = $1`, userID, tenantA); err != nil {
		t.Fatalf("set default tenant: %v", err)
	}

	var entityA string
	if err := conn.Get(&entityA, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantA, "entity-a-"+suffix); err != nil {
		t.Fatalf("insert entity a: %v", err)
	}
	var entityB string
	if err := conn.Get(&entityB, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantB, "entity-b-"+suffix); err != nil {
		t.Fatalf("insert entity b: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(`DELETE FROM entity_users WHERE entity_id = $1 OR entity_id = $2`, entityA, entityB)
		_, _ = conn.Exec(`DELETE FROM entities WHERE id = $1 OR id = $2`, entityA, entityB)
	}()

	if _, err := conn.Exec(`INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityA); err != nil {
		t.Fatalf("insert entity membership: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantA, time.Hour)

	objects := storage.NewLocalStore(os.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), queue.NewDBQueue(conn))
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "/entities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entities []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &entities); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if entities[0]["id"].(string) != entityA {
		t.Fatalf("expected entityA")
	}

	req = httptest.NewRequest(http.MethodGet, "/entities/"+entityB+"/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
