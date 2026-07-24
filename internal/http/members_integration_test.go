//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestAddMemberByEmail(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entity_users")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	teammateEmail := "teammate-" + testutil.UniqueSuffix() + "@example.com"
	teammate, err := stores.Users.Create(ctx, teammateEmail, "hash", false)
	if err != nil {
		t.Fatalf("create teammate: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/entities/"+entityID+"/members", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, req)
		return w
	}

	// Add by email resolves to the teammate's user id.
	w := post(`{"email":"` + teammateEmail + `","role":"user"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add by email status = %d, want 201: %s", w.Code, w.Body.String())
	}
	var member struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}
	if member.UserID != teammate.ID {
		t.Fatalf("member user_id = %q, want %q", member.UserID, teammate.ID)
	}

	// An unknown email is a clean 404.
	if w := post(`{"email":"nobody-` + testutil.UniqueSuffix() + `@example.com","role":"user"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown email status = %d, want 404: %s", w.Code, w.Body.String())
	}
}
