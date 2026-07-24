package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
)

func TestAuthRequiredRejectsMissingTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", func() time.Time { return time.Now() })
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	// Issue token with empty tenant id (should be rejected by middleware).
	tok, err := tokens.Issue("user-1", "", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	r := gin.New()
	r.GET("/secure", AuthRequired(tokens), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthRequiredAcceptsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := auth.NewTokenService("test-secret-32-bytes-exactly----!", func() time.Time { return time.Now() })
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	tok, err := tokens.Issue("user-1", "tenant-1", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	r := gin.New()
	r.GET("/secure", AuthRequired(tokens), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
