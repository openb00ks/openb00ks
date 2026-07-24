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
	"github.com/openb00ks/openb00ks/internal/models"
)

type meUserStore struct {
	user models.User
}

func (m meUserStore) Create(context.Context, string, string, bool) (models.User, error) {
	return models.User{}, nil
}
func (m meUserStore) GetByID(_ context.Context, _ string) (models.User, error) { return m.user, nil }
func (m meUserStore) GetByEmail(_ context.Context, _ string) (models.User, error) {
	return m.user, nil
}
func (m meUserStore) List(context.Context, int) ([]models.User, error)       { return nil, nil }
func (m meUserStore) Count(context.Context) (int, error)                     { return 0, nil }
func (m meUserStore) SetDefaultTenant(context.Context, string, string) error { return nil }

func TestHandleMeReturnsIdentityWithoutHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/me", nil)
	c.Set(string(userIDKey), "user-42")

	hc := &HandlerContext{users: meUserStore{user: models.User{
		ID:           "user-42",
		Email:        "owner@example.com",
		IsAdmin:      true,
		PasswordHash: "supersecret-hash",
		CreatedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}}}
	hc.handleMe(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["email"] != "owner@example.com" {
		t.Errorf("email = %v, want owner@example.com", body["email"])
	}
	if body["is_admin"] != true {
		t.Errorf("is_admin = %v, want true", body["is_admin"])
	}
	if _, leaked := body["password_hash"]; leaked {
		t.Error("response leaked password_hash key")
	}
	if strings.Contains(w.Body.String(), "supersecret-hash") {
		t.Error("response body contains the password hash")
	}
}
