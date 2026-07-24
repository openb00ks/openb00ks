//go:build integration

package httpapi

import (
	"bytes"
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

func TestRegisterSuccessReturnsSessionAndAccessesAuthedEndpoints(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "users")

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, true)

	server := newRegisterTestServer(t, conn)
	suffix := testutil.UniqueSuffix()
	email := "register-" + suffix + "@test.local"

	body, _ := json.Marshal(map[string]any{
		"email":       email,
		"password":    "secret123",
		"tenant_name": "Tenant " + suffix,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal register: %v", err)
	}
	if resp.Token == "" || resp.RefreshToken == "" {
		t.Fatalf("expected access and refresh tokens")
	}
	if resp.TokenType != "Bearer" {
		t.Fatalf("expected bearer token type")
	}
	if resp.ExpiresIn <= 0 || resp.RefreshExpiresIn <= 0 {
		t.Fatalf("expected positive expirations")
	}
	if resp.TenantID == "" {
		t.Fatalf("expected tenant id")
	}
	defer cleanupRegisteredUser(t, conn, email, resp.TenantID)

	req = httptest.NewRequest(http.MethodGet, "/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tenants []tenantResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tenants); err != nil {
		t.Fatalf("unmarshal tenants: %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(tenants))
	}
	if tenants[0].ID != resp.TenantID {
		t.Fatalf("expected tenant %s, got %s", resp.TenantID, tenants[0].ID)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "users")

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, true)

	suffix := testutil.UniqueSuffix()
	email := "duplicate-" + suffix + "@test.local"
	hash, _ := auth.HashPassword("secret123")
	if _, err := conn.Exec(`INSERT INTO users (email, password_hash) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING`, email, hash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	tenantName := "Tenant " + suffix

	server := newRegisterTestServer(t, conn)
	body, _ := json.Marshal(map[string]any{
		"email":       email,
		"password":    "secret123",
		"tenant_name": tenantName,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal duplicate error: %v", err)
	}
	if resp["error"] != "EMAIL_ALREADY_EXISTS" {
		t.Fatalf("expected EMAIL_ALREADY_EXISTS, got %q", resp["error"])
	}

	// Verify the failed registration didn't create a new tenant — check by name
	// rather than global COUNT(*) which is unstable under parallel test cleanup.
	var orphanCount int
	if err := conn.Get(&orphanCount, `SELECT COUNT(*) FROM tenants WHERE name = $1`, tenantName); err != nil {
		t.Fatalf("count orphan tenants: %v", err)
	}
	if orphanCount != 0 {
		t.Fatalf("expected no tenant created for duplicate email, got %d with name %q", orphanCount, tenantName)
	}
}

func TestRegisterRejectsMissingFields(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, true)

	server := newRegisterTestServer(t, conn)
	body, _ := json.Marshal(map[string]any{
		"email": "missing@test.local",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal missing fields error: %v", err)
	}
	if resp["error"] != "MISSING_FIELDS" {
		t.Fatalf("expected MISSING_FIELDS, got %q", resp["error"])
	}
}

func TestRegisterRequiresSetupCompletion(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, false)

	server := newRegisterTestServer(t, conn)
	body, _ := json.Marshal(map[string]any{
		"email":    "blocked@test.local",
		"password": "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal setup required error: %v", err)
	}
	if resp["error"] != "SETUP_REQUIRED" {
		t.Fatalf("expected SETUP_REQUIRED, got %q", resp["error"])
	}
}

func TestRegisterDisabledWhenPublicRegistrationOff(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, true)

	server := newRegisterTestServerWithMode(t, conn, false)
	body, _ := json.Marshal(map[string]any{
		"email":    "blocked@test.local",
		"password": "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal disabled error: %v", err)
	}
	if resp["error"] != "REGISTRATION_DISABLED" {
		t.Fatalf("expected REGISTRATION_DISABLED, got %q", resp["error"])
	}
}

func TestRegisterTrimsEmailAndAllowsLoginWithWhitespace(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "users")

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	setSetupCompleteForRegisterTests(t, conn, true)

	server := newRegisterTestServer(t, conn)
	suffix := testutil.UniqueSuffix()
	email := "trimmed-" + suffix + "@test.local"
	spacedEmail := "  " + email + "  "

	body, _ := json.Marshal(map[string]any{
		"email":       spacedEmail,
		"password":    "secret123",
		"tenant_name": "Tenant " + suffix,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var registerResp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &registerResp); err != nil {
		t.Fatalf("unmarshal register: %v", err)
	}
	defer cleanupRegisteredUser(t, conn, email, registerResp.TenantID)

	var storedEmail string
	if err := conn.Get(&storedEmail, `SELECT email FROM users WHERE email = $1`, email); err != nil {
		t.Fatalf("lookup stored email: %v", err)
	}
	if storedEmail != email {
		t.Fatalf("expected trimmed email %q, got %q", email, storedEmail)
	}

	loginBody, _ := json.Marshal(map[string]any{
		"email":    spacedEmail,
		"password": "secret123",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func newRegisterTestServer(t *testing.T, conn *db.DB) *Server {
	t.Helper()
	return newRegisterTestServerWithMode(t, conn, true)
}

func newRegisterTestServerWithMode(t *testing.T, conn *db.DB, publicRegistrationEnabled bool) *Server {
	t.Helper()
	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 24*time.Hour, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{
		PublicRegistrationEnabled: publicRegistrationEnabled,
	})
	hc.SetStores(db.NewStores(conn), nil)
	gin.SetMode(gin.TestMode)
	return NewServer(hc)
}

func setSetupCompleteForRegisterTests(t *testing.T, conn *db.DB, complete bool) {
	t.Helper()
	if _, err := conn.Exec(`
		INSERT INTO system_settings (id, setup_complete, setup_completed_at, settings_json, updated_at)
		VALUES (1, $1, CASE WHEN $1 THEN now() ELSE NULL END, '{}'::jsonb, now())
		ON CONFLICT (id) DO UPDATE
		SET setup_complete = EXCLUDED.setup_complete,
			setup_completed_at = EXCLUDED.setup_completed_at,
			settings_json = EXCLUDED.settings_json,
			updated_at = now()
	`, complete); err != nil {
		t.Fatalf("set system settings: %v", err)
	}
}

func cleanupRegisteredUser(t *testing.T, conn *db.DB, email, tenantID string) {
	t.Helper()
	var userID string
	if err := conn.Get(&userID, `SELECT id FROM users WHERE email = $1`, email); err != nil {
		t.Fatalf("lookup user: %v", err)
	}
	_, _ = conn.Exec(`DELETE FROM users WHERE id = $1`, userID)
	if tenantID != "" {
		_, _ = conn.Exec(`DELETE FROM tenants WHERE id = $1`, tenantID)
	}
}
