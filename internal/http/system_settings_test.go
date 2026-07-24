package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/suggest"
)

type readyOK struct{}

func (readyOK) Ready(context.Context) error {
	return nil
}

type fakeAdminChecker struct {
	allowed map[string]bool
}

func (f *fakeAdminChecker) IsAdmin(_ context.Context, userID string) (bool, error) {
	if f == nil || f.allowed == nil {
		return false, nil
	}
	return f.allowed[userID], nil
}

type fakeSystemSettingsStateStore struct {
	current db.SystemSettings
}

func (f *fakeSystemSettingsStateStore) Get(_ context.Context) (db.SystemSettings, error) {
	return f.current, nil
}

func (f *fakeSystemSettingsStateStore) SetSetupComplete(_ context.Context, _ time.Time) (db.SystemSettings, error) {
	f.current.SetupComplete = true
	return f.current, nil
}

func (f *fakeSystemSettingsStateStore) UpsertSettings(_ context.Context, settingsJSON json.RawMessage) (db.SystemSettings, error) {
	if settingsJSON == nil {
		settingsJSON = json.RawMessage(`{}`)
	}
	f.current.SettingsJSON = append(json.RawMessage(nil), settingsJSON...)
	if f.current.UpdatedAt.IsZero() {
		f.current.UpdatedAt = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	} else {
		f.current.UpdatedAt = f.current.UpdatedAt.Add(time.Second)
	}
	return f.current, nil
}

func newSystemSettingsTestServer(t *testing.T, isAdmin bool, current db.SystemSettings) (*Server, *fakeSystemSettingsStateStore, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	token, err := tokens.Issue("user-1", "tenant-1", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	store := &fakeSystemSettingsStateStore{current: current}
	hc := NewHandlerContext(readyOK{}, tokens, time.Hour, 0, nil, suggest.Pricing{}, nil, NewReceiptHandler(0), SystemInfo{
		AIProvider:      "openai",
		AIModel:         "gpt-test",
		ReceiptStorage:  "local",
		ReceiptLocalDir: "./.data/receipts",
		ReceiptMaxBytes: 1024,
	})
	hc.tenantMembers = &fakeTenantMemberships{
		roles: map[string]map[string]models.Role{
			"tenant-1": {"user-1": models.RoleAdmin},
		},
	}
	hc.admin = &fakeAdminChecker{
		allowed: map[string]bool{"user-1": isAdmin},
	}
	hc.systemSettings = store

	return NewServer(hc), store, token
}

func TestSystemSettingsGetRequiresAdmin(t *testing.T) {
	server, _, token := newSystemSettingsTestServer(t, false, db.SystemSettings{})

	req := httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSystemSettingsPatchRequiresAdmin(t *testing.T) {
	server, _, token := newSystemSettingsTestServer(t, false, db.SystemSettings{})

	req := httptest.NewRequest(http.MethodPatch, "/settings/system", bytes.NewBufferString(`{"require_mfa":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSystemSettingsGetSuccess(t *testing.T) {
	server, _, token := newSystemSettingsTestServer(t, true, db.SystemSettings{
		SettingsJSON: json.RawMessage(`{"require_mfa":true,"enforce_session_timeout":false}`),
		UpdatedAt:    time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	})

	req := httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp systemSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Settings.RequireMFA {
		t.Fatal("expected require_mfa=true")
	}
	if resp.Settings.EnforceSessionTimeout {
		t.Fatal("expected enforce_session_timeout=false")
	}
	if resp.Integrations.AIProvider != "openai" || resp.Integrations.ReceiptStorage != "local" {
		t.Fatalf("unexpected integrations payload: %+v", resp.Integrations)
	}
	if resp.UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected updated_at %q", resp.UpdatedAt)
	}
}

func TestSystemSettingsPatchRejectsMalformedJSON(t *testing.T) {
	server, _, token := newSystemSettingsTestServer(t, true, db.SystemSettings{})

	req := httptest.NewRequest(http.MethodPatch, "/settings/system", bytes.NewBufferString(`{"require_mfa":`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSystemSettingsPatchPartialUpdatePreservesExistingValues(t *testing.T) {
	server, store, token := newSystemSettingsTestServer(t, true, db.SystemSettings{
		SettingsJSON: json.RawMessage(`{"require_mfa":true,"enforce_session_timeout":false}`),
	})

	req := httptest.NewRequest(http.MethodPatch, "/settings/system", bytes.NewBufferString(`{"enforce_session_timeout":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp systemSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Settings.RequireMFA || !resp.Settings.EnforceSessionTimeout {
		t.Fatalf("unexpected merged settings: %+v", resp.Settings)
	}

	var persisted systemSettingsData
	if err := json.Unmarshal(store.current.SettingsJSON, &persisted); err != nil {
		t.Fatalf("persisted json: %v", err)
	}
	if !persisted.RequireMFA || !persisted.EnforceSessionTimeout {
		t.Fatalf("unexpected persisted settings: %+v", persisted)
	}
}
