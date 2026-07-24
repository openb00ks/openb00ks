//go:build integration

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/queue"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestEntityCreateSeedsCashAccount(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "users")

	suffix := testutil.UniqueSuffix()
	email := "entity+" + strings.ReplaceAll(suffix, ".", "") + "@test.local"
	hash, _ := auth.HashPassword("testpass123")
	var userID string
	if err := conn.Get(&userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, email, hash); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var tenantID string
	if err := conn.Get(&tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if _, err := conn.Exec(`UPDATE users SET default_tenant_id = $2 WHERE id = $1`, userID, tenantID); err != nil {
		t.Fatalf("set default tenant: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantID, userID); err != nil {
		t.Fatalf("insert tenant membership: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(os.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), queue.NewDBQueue(conn))
	server := NewServer(hc)

	name := "Entity Cash Seed " + suffix
	body, _ := json.Marshal(map[string]string{"name": name})
	req := httptest.NewRequest(http.MethodPost, "/entities", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	entityID, _ := resp["id"].(string)
	if entityID == "" {
		t.Fatal("missing entity id")
	}

	var accountID string
	err = conn.Get(&accountID, `
		SELECT id
		FROM accounts
		WHERE entity_id = $1 AND lower(name) = 'cash'
		LIMIT 1
	`, entityID)
	if err != nil {
		t.Fatalf("expected cash account: %v", err)
	}
}
