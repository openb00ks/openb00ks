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

func TestSystemSettingsRoundTrip(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "system_settings")

	tenantID, userID, _, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	if _, err := conn.Exec(`UPDATE users SET is_admin = true WHERE id = $1`, userID); err != nil {
		t.Fatalf("set admin: %v", err)
	}

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()

	if _, err := conn.Exec(`
		INSERT INTO system_settings (id, setup_complete, settings_json, updated_at)
		VALUES (1, false, $1, now())
		ON CONFLICT (id) DO UPDATE
		SET setup_complete = EXCLUDED.setup_complete,
			settings_json = EXCLUDED.settings_json,
			updated_at = now()
	`, json.RawMessage(`{"require_mfa":true,"enforce_session_timeout":false}`)); err != nil {
		t.Fatalf("seed system settings: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{
		AIProvider:      "openai",
		AIModel:         "gpt-test",
		ReceiptStorage:  "local",
		ReceiptLocalDir: t.TempDir(),
		ReceiptMaxBytes: 10 * 1024 * 1024,
	})
	hc.SetStores(db.NewStores(conn), nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodPatch, "/settings/system", bytes.NewBufferString(`{"enforce_session_timeout":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var patched systemSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if !patched.Settings.RequireMFA || !patched.Settings.EnforceSessionTimeout {
		t.Fatalf("unexpected patched settings: %+v", patched.Settings)
	}

	req = httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var fetched systemSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if !fetched.Settings.RequireMFA || !fetched.Settings.EnforceSessionTimeout {
		t.Fatalf("unexpected fetched settings: %+v", fetched.Settings)
	}

	var stored json.RawMessage
	if err := conn.Get(&stored, `SELECT settings_json FROM system_settings WHERE id = 1`); err != nil {
		t.Fatalf("select settings_json: %v", err)
	}
	var persisted systemSettingsData
	if err := json.Unmarshal(stored, &persisted); err != nil {
		t.Fatalf("unmarshal stored settings: %v", err)
	}
	if !persisted.RequireMFA || !persisted.EnforceSessionTimeout {
		t.Fatalf("unexpected stored settings: %+v", persisted)
	}
}

func snapshotSystemSettings(t *testing.T, conn *db.DB) func() {
	t.Helper()

	var previous db.SystemSettingsRow
	err := conn.Get(&previous, `
		SELECT id, setup_complete, setup_completed_at, settings_json, updated_at
		FROM system_settings
		WHERE id = 1
	`)
	hadRow := err == nil
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("snapshot system_settings: %v", err)
	}

	return func() {
		if !hadRow {
			if _, restoreErr := conn.Exec(`DELETE FROM system_settings WHERE id = 1`); restoreErr != nil {
				t.Errorf("restore system_settings delete: %v", restoreErr)
			}
			return
		}

		var completedAt any
		if previous.SetupCompletedAt.Valid {
			completedAt = previous.SetupCompletedAt.Time
		}

		if _, restoreErr := conn.Exec(`
			INSERT INTO system_settings (id, setup_complete, setup_completed_at, settings_json, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE
			SET setup_complete = EXCLUDED.setup_complete,
				setup_completed_at = EXCLUDED.setup_completed_at,
				settings_json = EXCLUDED.settings_json,
				updated_at = EXCLUDED.updated_at
		`, previous.ID, previous.SetupComplete, completedAt, previous.SettingsJSON, previous.UpdatedAt); restoreErr != nil {
			t.Errorf("restore system_settings upsert: %v", restoreErr)
		}
	}
}
