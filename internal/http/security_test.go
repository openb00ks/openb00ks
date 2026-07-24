package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

// fakeRefreshTokenStore is an in-memory RefreshTokenStore for unit tests.
type fakeRefreshTokenStore struct {
	byHash  map[string]models.RefreshToken
	revoked map[string]bool
	nextID  int
}

func newFakeRefreshTokenStore() *fakeRefreshTokenStore {
	return &fakeRefreshTokenStore{
		byHash:  make(map[string]models.RefreshToken),
		revoked: make(map[string]bool),
	}
}

func (f *fakeRefreshTokenStore) Create(_ context.Context, userID, tenantID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
	f.nextID++
	rt := models.RefreshToken{
		ID:        itoa(f.nextID),
		UserID:    userID,
		TenantID:  tenantID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	f.byHash[tokenHash] = rt
	return rt, nil
}

func (f *fakeRefreshTokenStore) GetByHash(_ context.Context, tokenHash string) (models.RefreshToken, error) {
	rt, ok := f.byHash[tokenHash]
	if !ok {
		return models.RefreshToken{}, db.ErrNotFound
	}
	if f.revoked[rt.ID] {
		now := time.Now()
		rt.RevokedAt = &now
	}
	return rt, nil
}

func (f *fakeRefreshTokenStore) Revoke(_ context.Context, id string, _ time.Time) error {
	f.revoked[id] = true
	return nil
}

func (f *fakeRefreshTokenStore) RevokeIfActive(_ context.Context, id string, _ time.Time) (bool, error) {
	if f.revoked[id] {
		return false, nil
	}
	f.revoked[id] = true
	return true, nil
}

func (f *fakeRefreshTokenStore) RevokeAllForUser(_ context.Context, _ string, _ time.Time) error {
	return nil
}

// fakeUserMFAStore is an in-memory UserMFAStore for unit tests.
type fakeUserMFAStore struct {
	records map[string]db.UserMFA
}

func newFakeUserMFAStore() *fakeUserMFAStore {
	return &fakeUserMFAStore{records: make(map[string]db.UserMFA)}
}

func (f *fakeUserMFAStore) GetByUserID(_ context.Context, userID string) (db.UserMFA, error) {
	r, ok := f.records[userID]
	if !ok {
		return db.UserMFA{}, db.ErrNotFound
	}
	return r, nil
}

func (f *fakeUserMFAStore) UpsertEnrollment(_ context.Context, userID, secret string, hashes json.RawMessage) (db.UserMFA, error) {
	r := db.UserMFA{UserID: userID, Secret: secret, RecoveryCodeHashes: hashes}
	f.records[userID] = r
	return r, nil
}

func (f *fakeUserMFAStore) Enable(_ context.Context, userID string) (db.UserMFA, error) {
	r := f.records[userID]
	r.Enabled = true
	f.records[userID] = r
	return r, nil
}

func (f *fakeUserMFAStore) Disable(_ context.Context, userID string) (db.UserMFA, error) {
	r := f.records[userID]
	r.Enabled = false
	f.records[userID] = r
	return r, nil
}

func (f *fakeUserMFAStore) SetRecoveryCodeHashes(_ context.Context, userID string, hashes json.RawMessage) (db.UserMFA, error) {
	r := f.records[userID]
	r.RecoveryCodeHashes = hashes
	f.records[userID] = r
	return r, nil
}

// helpers

func newTokenService(t *testing.T) *auth.TokenService {
	t.Helper()
	svc, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", func() time.Time { return time.Now() })
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	return svc
}

func newRegisterHC(t *testing.T) *HandlerContext {
	t.Helper()
	tokens := newTokenService(t)
	return &HandlerContext{
		tokens:         tokens,
		refreshTokens:  newFakeRefreshTokenStore(),
		users:          &fakeUserStore{},
		tenants:        &fakeTenantStore{},
		tenantMembers:  &fakeTenantMembershipStore{},
		systemSettings: &fakeSystemSettingsStore{setupComplete: true},
		systemInfo:     SystemInfo{PublicRegistrationEnabled: true},
		jwtTTL:         time.Hour,
		refreshTTL:     24 * time.Hour,
	}
}

// --- password minimum length ---

func TestRegisterRejectsShortPassword(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	hc := newRegisterHC(t)
	r := gin.New()
	r.POST("/auth/register", hc.handleRegister)

	body := `{"email":"user@test.local","password":"short","tenant_name":"Acme"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "PASSWORD_TOO_SHORT" {
		t.Fatalf("expected PASSWORD_TOO_SHORT, got %q", resp["error"])
	}
}

func TestRegisterAcceptsMinimumLengthPassword(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	hc := newRegisterHC(t)
	r := gin.New()
	r.POST("/auth/register", hc.handleRegister)

	body := `{"email":"user2@test.local","password":"exactly8","tenant_name":"Acme"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		users:          &fakeUserStore{},
		tenants:        &fakeTenantStore{},
		tenantMembers:  &fakeTenantMembershipStore{},
		entities:       &fakeSetupEntityStore{},
		systemSettings: &fakeSystemSettingsStore{setupComplete: false},
	}
	r := gin.New()
	r.POST("/setup", hc.handleSetup)

	body := `{"tenant_name":"Acme","admin_email":"admin@test.local","admin_password":"short","default_entity_name":"Acme LLC"}`
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "PASSWORD_TOO_SHORT" {
		t.Fatalf("expected PASSWORD_TOO_SHORT, got %q", resp["error"])
	}
}

// --- MFA disable requires TOTP code ---

func TestMFADisableRejectsMissingCode(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	tokens := newTokenService(t)
	mfaStore := newFakeUserMFAStore()
	mfaStore.records["user-1"] = db.UserMFA{UserID: "user-1", Secret: "JBSWY3DPEHPK3PXP", Enabled: true}

	hc := &HandlerContext{
		tokens:  tokens,
		userMFA: mfaStore,
	}
	r := gin.New()
	tok, err := tokens.Issue("user-1", "tenant-1", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	r.DELETE("/mfa", AuthRequired(tokens), hc.handleMFADisable)

	req := httptest.NewRequest(http.MethodDelete, "/mfa", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "MISSING_FIELDS" {
		t.Fatalf("expected MISSING_FIELDS, got %q", resp["error"])
	}
}

func TestMFADisableRejectsWrongCode(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	tokens := newTokenService(t)
	mfaStore := newFakeUserMFAStore()
	mfaStore.records["user-1"] = db.UserMFA{UserID: "user-1", Secret: "JBSWY3DPEHPK3PXP", Enabled: true}

	hc := &HandlerContext{
		tokens:  tokens,
		userMFA: mfaStore,
	}
	r := gin.New()
	tok, err := tokens.Issue("user-1", "tenant-1", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	r.DELETE("/mfa", AuthRequired(tokens), hc.handleMFADisable)

	req := httptest.NewRequest(http.MethodDelete, "/mfa", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "INVALID_MFA_CODE" {
		t.Fatalf("expected INVALID_MFA_CODE, got %q", resp["error"])
	}
}

// --- refresh token atomic revocation ---

func TestRefreshRejectsAlreadyRevokedToken(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	tokens := newTokenService(t)
	rtStore := newFakeRefreshTokenStore()

	// Pre-insert a token that is already revoked.
	rawToken := "raw-token-value"
	hash := auth.HashRefreshToken(rawToken)
	now := time.Now()
	rt := models.RefreshToken{
		ID:        "rt-1",
		UserID:    "user-1",
		TenantID:  "tenant-1",
		TokenHash: hash,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	rtStore.byHash[hash] = rt
	rtStore.revoked["rt-1"] = true // already revoked

	hc := &HandlerContext{
		tokens:        tokens,
		refreshTokens: rtStore,
	}
	r := gin.New()
	r.POST("/auth/refresh", hc.handleRefresh)

	body := `{"refresh_token":"` + rawToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "INVALID_REFRESH_TOKEN" {
		t.Fatalf("expected INVALID_REFRESH_TOKEN, got %q", resp["error"])
	}
}

func TestRefreshConcurrentRequestRejected(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	tokens := newTokenService(t)
	rtStore := newFakeRefreshTokenStore()

	// Pre-insert a valid token.
	rawToken := "concurrent-token-value"
	hash := auth.HashRefreshToken(rawToken)
	now := time.Now()
	rt := models.RefreshToken{
		ID:        "rt-2",
		UserID:    "user-1",
		TenantID:  "tenant-1",
		TokenHash: hash,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	rtStore.byHash[hash] = rt

	hc := &HandlerContext{
		tokens:        tokens,
		refreshTokens: rtStore,
		users:         &fakeUserStore{count: 1},
		tenantMembers: &fakeTenantMembershipStore{},
		jwtTTL:        time.Hour,
		refreshTTL:    24 * time.Hour,
	}
	r := gin.New()
	r.POST("/auth/refresh", hc.handleRefresh)

	doRefresh := func() *httptest.ResponseRecorder {
		body := `{"refresh_token":"` + rawToken + `"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// First request must succeed.
	w1 := doRefresh()
	if w1.Code != http.StatusOK {
		t.Fatalf("first refresh: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	// Second request with the same token must be rejected (RevokeIfActive returns false).
	w2 := doRefresh()
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("second refresh: expected 401, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "INVALID_REFRESH_TOKEN" {
		t.Fatalf("expected INVALID_REFRESH_TOKEN, got %q", resp["error"])
	}
}
