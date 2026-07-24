package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type fakeResetUserStore struct {
	user models.User
}

func (f *fakeResetUserStore) Create(context.Context, string, string, bool) (models.User, error) {
	return models.User{}, nil
}

func (f *fakeResetUserStore) GetByID(_ context.Context, id string) (models.User, error) {
	if f.user.ID == "" {
		f.user.ID = id
	}
	return f.user, nil
}

func (f *fakeResetUserStore) GetByEmail(_ context.Context, _ string) (models.User, error) {
	return f.user, nil
}

func (f *fakeResetUserStore) List(context.Context, int) ([]models.User, error) {
	return nil, nil
}

func (f *fakeResetUserStore) Count(context.Context) (int, error) {
	return 0, nil
}

func (f *fakeResetUserStore) SetDefaultTenant(context.Context, string, string) error {
	return nil
}

type fakeResetMFAStore struct {
	current db.UserMFA
	calls   int
}

func (f *fakeResetMFAStore) GetByUserID(_ context.Context, userID string) (db.UserMFA, error) {
	if f.current.UserID == "" {
		return db.UserMFA{}, db.ErrNotFound
	}
	if f.current.UserID != userID {
		return db.UserMFA{}, db.ErrNotFound
	}
	return f.current, nil
}

func (f *fakeResetMFAStore) UpsertEnrollment(context.Context, string, string, json.RawMessage) (db.UserMFA, error) {
	return db.UserMFA{}, nil
}

func (f *fakeResetMFAStore) Enable(context.Context, string) (db.UserMFA, error) {
	return db.UserMFA{}, nil
}

func (f *fakeResetMFAStore) Disable(_ context.Context, userID string) (db.UserMFA, error) {
	f.calls++
	f.current = db.UserMFA{UserID: userID}
	return f.current, nil
}

func (f *fakeResetMFAStore) SetRecoveryCodeHashes(context.Context, string, json.RawMessage) (db.UserMFA, error) {
	return db.UserMFA{}, nil
}

func TestResetUserMFA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeResetMFAStore{current: db.UserMFA{UserID: "user-1", Secret: "secret", Enabled: true}}
	hc := &HandlerContext{
		users:   &fakeResetUserStore{user: models.User{ID: "user-1", Email: "admin@test.local"}},
		userMFA: store,
	}

	r := gin.New()
	r.POST("/users/:id/mfa/reset", hc.handleResetUserMFA)

	req := httptest.NewRequest(http.MethodPost, "/users/user-1/mfa/reset", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if store.calls != 1 {
		t.Fatalf("expected disable to be called once, got %d", store.calls)
	}
	if store.current.Secret != "" || store.current.Enabled {
		t.Fatalf("expected MFA to be cleared, got %+v", store.current)
	}
}
