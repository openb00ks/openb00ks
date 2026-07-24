package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type fakeTenantMemberships struct {
	roles map[string]map[string]models.Role
}

func (f *fakeTenantMemberships) ListForUser(ctx context.Context, userID string, limit int) ([]models.TenantMembership, error) {
	return nil, nil
}

func (f *fakeTenantMemberships) Create(ctx context.Context, tenantID, userID string, role models.Role) (models.TenantMembership, error) {
	return models.TenantMembership{}, nil
}

func (f *fakeTenantMemberships) GetRole(ctx context.Context, tenantID, userID string) (models.Role, error) {
	if f == nil {
		return "", db.ErrNotFound
	}
	if f.roles == nil {
		return "", db.ErrNotFound
	}
	users := f.roles[tenantID]
	if users == nil {
		return "", db.ErrNotFound
	}
	role, ok := users[userID]
	if !ok {
		return "", db.ErrNotFound
	}
	return role, nil
}

type fakeEntityStore struct {
	roles    map[string]map[string]models.Role
	entities map[string]models.Entity
}

func (f *fakeEntityStore) ListForUser(ctx context.Context, tenantID, userID string, limit int) ([]models.Entity, error) {
	return nil, nil
}

func (f *fakeEntityStore) CreateWithOwner(ctx context.Context, tenantID, userID, name, suggestionContext string) (models.Entity, error) {
	return models.Entity{}, nil
}

func (f *fakeEntityStore) Update(ctx context.Context, tenantID, entityID string, name *string, suggestionContext *string, fiscalYearStartMonth, fiscalYearStartDay *int) (models.Entity, error) {
	return models.Entity{}, nil
}

func (f *fakeEntityStore) Delete(ctx context.Context, tenantID, entityID string) error {
	return nil
}

func (f *fakeEntityStore) GetRole(ctx context.Context, tenantID, userID, entityID string) (models.Role, error) {
	if f == nil {
		return "", db.ErrNotFound
	}
	if f.roles == nil {
		return "", db.ErrNotFound
	}
	entities := f.roles[tenantID]
	if entities == nil {
		return "", db.ErrNotFound
	}
	role, ok := entities[entityID+"|"+userID]
	if !ok {
		return "", db.ErrNotFound
	}
	return role, nil
}

func (f *fakeEntityStore) GetByID(ctx context.Context, entityID string) (models.Entity, error) {
	if f == nil || f.entities == nil {
		return models.Entity{}, db.ErrNotFound
	}
	entity, ok := f.entities[entityID]
	if !ok {
		return models.Entity{}, db.ErrNotFound
	}
	return entity, nil
}

type fakeReceiptStore struct {
	entityByReceipt map[string]string
	receipts        map[string]models.Receipt
}

func (f *fakeReceiptStore) Create(ctx context.Context, entityID, storageKey, contentType, status, kind, originalName string, sizeBytes int64, totalCents int64) (models.Receipt, error) {
	return models.Receipt{}, nil
}

func (f *fakeReceiptStore) GetByID(ctx context.Context, id string) (models.Receipt, error) {
	if f != nil && f.receipts != nil {
		if receipt, ok := f.receipts[id]; ok {
			return receipt, nil
		}
	}
	return models.Receipt{}, db.ErrNotFound
}

func (f *fakeReceiptStore) GetEntityID(ctx context.Context, id string) (string, error) {
	if f == nil || f.entityByReceipt == nil {
		return "", db.ErrNotFound
	}
	entityID, ok := f.entityByReceipt[id]
	if !ok {
		return "", db.ErrNotFound
	}
	return entityID, nil
}

func (f *fakeReceiptStore) UpdateStatus(ctx context.Context, id, status string) error {
	return nil
}

func (f *fakeReceiptStore) UpdateResolvedVendorID(ctx context.Context, id, vendorID string) error {
	return nil
}

func (f *fakeReceiptStore) List(ctx context.Context, entityID string, status string, limit int) ([]models.Receipt, error) {
	return nil, nil
}

func (f *fakeReceiptStore) ListByKind(ctx context.Context, entityID, kind, status string, limit int) ([]models.Receipt, error) {
	return nil, nil
}

func withAuth(userID, tenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID != "" {
			c.Set(string(userIDKey), userID)
		}
		if tenantID != "" {
			c.Set(string(tenantIDKey), tenantID)
		}
		c.Next()
	}
}

func TestRequireTenantMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		tenantMembers: &fakeTenantMemberships{
			roles: map[string]map[string]models.Role{
				"tenant-1": {"user-1": models.RoleAdmin},
			},
		},
	}

	r := gin.New()
	r.GET("/ok", withAuth("user-1", "tenant-1"), hc.requireTenantMembership(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	r = gin.New()
	r.GET("/deny", withAuth("user-1", "tenant-2"), hc.requireTenantMembership(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req = httptest.NewRequest(http.MethodGet, "/deny", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRequireEntityRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		entities: &fakeEntityStore{
			roles: map[string]map[string]models.Role{
				"tenant-1": {"entity-1|user-1": models.RoleAdmin},
			},
		},
	}

	r := gin.New()
	r.GET("/ok", withAuth("user-1", "tenant-1"), hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok?entity_id=entity-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/ok?entity_id=entity-2", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequireOptionalEntityRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		entities: &fakeEntityStore{
			roles: map[string]map[string]models.Role{
				"tenant-1": {"entity-1|user-1": models.RoleAdmin},
			},
		},
	}

	r := gin.New()
	r.PATCH("/prefs", withAuth("user-1", "tenant-1"), hc.requireOptionalEntityRole(adminRoles(), "default_entity_id"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	body, _ := json.Marshal(map[string]string{"theme": "system"})
	req := httptest.NewRequest(http.MethodPatch, "/prefs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body, _ = json.Marshal(map[string]string{"default_entity_id": "entity-2"})
	req = httptest.NewRequest(http.MethodPatch, "/prefs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRequireReceiptsRoleFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		receiptStore: &fakeReceiptStore{entityByReceipt: map[string]string{
			"receipt-a": "entity-1",
			"receipt-b": "entity-2",
		}},
		entities: &fakeEntityStore{
			roles: map[string]map[string]models.Role{
				"tenant-1": {"entity-1|user-1": models.RoleAdmin},
			},
		},
	}

	r := gin.New()
	r.POST("/batch", withAuth("user-1", "tenant-1"), hc.requireReceiptsRole(memberRoles(), receiptIDsFromBody("receipt_ids")), func(c *gin.Context) {
		ids, _ := EntityIDs(c)
		c.JSON(http.StatusOK, gin.H{"allowed": ids})
	})

	payload, _ := json.Marshal(map[string][]string{"receipt_ids": {"receipt-a", "receipt-b"}})
	req := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string][]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	allowed := resp["allowed"]
	if len(allowed) != 1 || allowed[0] != "receipt-a" {
		t.Fatalf("expected only receipt-a, got %v", allowed)
	}
}

func TestRequireTenantAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		tenantMembers: &fakeTenantMemberships{
			roles: map[string]map[string]models.Role{
				"tenant-1": {"user-1": models.RoleAdmin},
			},
		},
	}

	r := gin.New()
	r.POST("/tenants/:id/switch", withAuth("user-1", "tenant-1"), hc.requireTenantAccess("id"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/tenants/tenant-1/switch", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/tenants/tenant-2/switch", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRequireEntityRoleBadJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{entities: &fakeEntityStore{}}
	r := gin.New()
	r.POST("/bad", withAuth("user-1", "tenant-1"), hc.requireEntityRole(memberRoles(), entityIDFromJSON("entity_id")), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/bad", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequireEntityRoleMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{entities: &fakeEntityStore{}}
	r := gin.New()
	r.GET("/noauth", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/noauth?entity_id=entity-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequireReceiptsRoleMissingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{receiptStore: &fakeReceiptStore{}, entities: &fakeEntityStore{}}
	r := gin.New()
	r.POST("/batch", withAuth("user-1", "tenant-1"), hc.requireReceiptsRole(memberRoles(), receiptIDsFromBody("receipt_ids")), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/batch", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
