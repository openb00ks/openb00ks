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

func TestEntityTaxSettingsGetAndUpdate(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entity_tax_settings")

	suffix := testutil.UniqueSuffix()
	email := "tax-settings+" + strings.ReplaceAll(suffix, ".", "") + "@test.local"
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
	var entityID string
	if err := conn.Get(&entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "Entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), queue.NewDBQueue(conn))
	server := NewServer(hc)

	req := httptest.NewRequest(http.MethodGet, "/entities/"+entityID+"/tax-settings?year=2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var empty map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty response: %v", err)
	}
	if year, ok := empty["tax_year"].(float64); !ok || int(year) != 2026 {
		t.Fatalf("expected tax year 2026, got %#v", empty["tax_year"])
	}

	body, _ := json.Marshal(map[string]any{
		"tax_year":                           2026,
		"home_office_sqft":                   250,
		"home_total_sqft":                    1000,
		"cell_phone_business_use_percent":    75,
		"home_internet_business_use_percent": 60,
	})
	req = httptest.NewRequest(http.MethodPatch, "/entities/"+entityID+"/tax-settings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if got := int(updated["home_utilities_business_use_percent"].(float64)); got != 25 {
		t.Fatalf("expected utilities ratio 25, got %d", got)
	}

	req = httptest.NewRequest(http.MethodPatch, "/entities/"+entityID+"/tax-settings", bytes.NewReader([]byte(`{"tax_year":2026,"home_office_sqft":1200,"home_total_sqft":1000}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid home office ratio, got %d", w.Code)
	}
}
