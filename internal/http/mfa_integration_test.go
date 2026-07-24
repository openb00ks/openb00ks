//go:build integration

package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestMFASetupAndLoginChallengeRoundTrip(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "user_mfa")

	_, userID, _, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	var email string
	if err := conn.Get(&email, `SELECT email FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("lookup email: %v", err)
	}

	restoreSystemSettings := snapshotSystemSettings(t, conn)
	defer restoreSystemSettings()
	if _, err := conn.Exec(`
		INSERT INTO system_settings (id, setup_complete, settings_json, updated_at)
		VALUES (1, true, '{}'::jsonb, now())
		ON CONFLICT (id) DO UPDATE
		SET setup_complete = EXCLUDED.setup_complete,
			settings_json = EXCLUDED.settings_json,
			updated_at = now()
	`); err != nil {
		t.Fatalf("seed system settings: %v", err)
	}

	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 24*time.Hour, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(db.NewStores(conn), nil)
	gin.SetMode(gin.TestMode)
	server := NewServer(hc)

	loginToken := loginAndGetToken(t, server, email, "testpass123")
	if loginToken == "" {
		t.Fatal("expected initial login token")
	}

	req := httptest.NewRequest(http.MethodGet, "/me/mfa", nil)
	req.Header.Set("Authorization", "Bearer "+loginToken)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/me/mfa/setup", nil)
	req.Header.Set("Authorization", "Bearer "+loginToken)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var setupResp mfaStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &setupResp); err != nil {
		t.Fatalf("unmarshal setup: %v", err)
	}
	if !setupResp.Configured || setupResp.Secret == "" || setupResp.URI == "" {
		t.Fatalf("unexpected setup response: %+v", setupResp)
	}
	if len(setupResp.RecoveryCodes) == 0 {
		t.Fatal("expected recovery codes")
	}

	now := time.Now().UTC()
	code := auth.GenerateTOTPCode(setupResp.Secret, now)
	req = httptest.NewRequest(http.MethodPost, "/me/mfa/confirm", bytes.NewBufferString(`{"code":"`+code+`"}`))
	req.Header.Set("Authorization", "Bearer "+loginToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	challengeToken, err := loginForMFAChallenge(t, server, email, "testpass123")
	if err != nil {
		t.Fatal(err)
	}
	if challengeToken == "" {
		t.Fatal("expected challenge token")
	}

	// The confirm step above consumed the current TOTP step, and the server records
	// used steps to reject replay. Use a code from the next step for the login
	// challenge — still within the server's ±MFALeeway validation window, but a
	// distinct step counter, so it isn't rejected as already used.
	loginCode := auth.GenerateTOTPCode(setupResp.Secret, now.Add(auth.MFAStep))
	req = httptest.NewRequest(http.MethodPost, "/auth/login/mfa", bytes.NewBufferString(`{"challenge_token":"`+challengeToken+`","code":"`+loginCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var loginResp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal mfa login: %v", err)
	}
	if loginResp.Token == "" || loginResp.RefreshToken == "" {
		t.Fatalf("expected issued session, got %+v", loginResp)
	}
	if loginResp.MFARequired {
		t.Fatal("expected MFA challenge to complete")
	}

	challengeToken, err = loginForMFAChallenge(t, server, email, "testpass123")
	if err != nil {
		t.Fatal(err)
	}
	recoveryCode := setupResp.RecoveryCodes[0]
	req = httptest.NewRequest(http.MethodPost, "/auth/login/mfa", bytes.NewBufferString(`{"challenge_token":"`+challengeToken+`","code":"`+recoveryCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from recovery code login, got %d: %s", w.Code, w.Body.String())
	}

	challengeToken, err = loginForMFAChallenge(t, server, email, "testpass123")
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/login/mfa", bytes.NewBufferString(`{"challenge_token":"`+challengeToken+`","code":"`+recoveryCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after consuming recovery code, got %d: %s", w.Code, w.Body.String())
	}

}

func loginAndGetToken(t *testing.T, server *Server, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login: %v", err)
	}
	return resp.Token
}

func loginForMFAChallenge(t *testing.T, server *Server, email, password string) (string, error) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return "", fmt.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return "", err
	}
	if !resp.MFARequired || resp.ChallengeToken == "" {
		return "", fmt.Errorf("expected MFA challenge, got %+v", resp)
	}
	return resp.ChallengeToken, nil
}
