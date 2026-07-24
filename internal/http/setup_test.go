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
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type fakeUserStore struct {
	count         int
	nextID        int
	defaultTenant map[string]string
	byEmail       map[string]models.User
}

func (f *fakeUserStore) Create(_ context.Context, email, passwordHash string, isAdmin bool) (models.User, error) {
	_ = passwordHash
	f.count++
	f.nextID++
	if f.defaultTenant == nil {
		f.defaultTenant = make(map[string]string)
	}
	return models.User{
		ID:           "user-" + itoa(f.nextID),
		Email:        email,
		IsAdmin:      isAdmin,
		PasswordHash: "",
	}, nil
}

func (f *fakeUserStore) GetByID(_ context.Context, id string) (models.User, error) {
	return models.User{ID: id}, nil
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (models.User, error) {
	if f.byEmail != nil {
		if user, ok := f.byEmail[email]; ok {
			return user, nil
		}
	}
	return models.User{}, db.ErrNotFound
}

func (f *fakeUserStore) List(_ context.Context, limit int) ([]models.User, error) {
	_ = limit
	return nil, nil
}

func (f *fakeUserStore) Count(_ context.Context) (int, error) {
	return f.count, nil
}

func (f *fakeUserStore) SetDefaultTenant(_ context.Context, userID, tenantID string) error {
	if f.defaultTenant == nil {
		f.defaultTenant = make(map[string]string)
	}
	f.defaultTenant[userID] = tenantID
	return nil
}

type fakeTenantStore struct {
	count  int
	nextID int
}

func (f *fakeTenantStore) Create(_ context.Context, name string) (models.Tenant, error) {
	f.count++
	f.nextID++
	return models.Tenant{ID: "tenant-" + itoa(f.nextID), Name: name}, nil
}

func (f *fakeTenantStore) GetByID(_ context.Context, id string) (models.Tenant, error) {
	return models.Tenant{ID: id}, nil
}

func (f *fakeTenantStore) Count(_ context.Context) (int, error) {
	return f.count, nil
}

type fakeTenantMembershipStore struct {
	created []models.TenantMembership
}

func (f *fakeTenantMembershipStore) ListForUser(_ context.Context, userID string, limit int) ([]models.TenantMembership, error) {
	_ = userID
	_ = limit
	return nil, nil
}

func (f *fakeTenantMembershipStore) Create(_ context.Context, tenantID, userID string, role models.Role) (models.TenantMembership, error) {
	member := models.TenantMembership{
		ID:       "tm-" + tenantID + "-" + userID,
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}
	f.created = append(f.created, member)
	return member, nil
}

func (f *fakeTenantMembershipStore) GetRole(_ context.Context, tenantID, userID string) (models.Role, error) {
	for _, member := range f.created {
		if member.TenantID == tenantID && member.UserID == userID {
			return member.Role, nil
		}
	}
	return "", nil
}

type fakeSetupEntityStore struct {
	created []models.Entity
	nextID  int
}

func (f *fakeSetupEntityStore) ListForUser(_ context.Context, tenantID, userID string, limit int) ([]models.Entity, error) {
	_ = tenantID
	_ = userID
	_ = limit
	return nil, nil
}

func (f *fakeSetupEntityStore) CreateWithOwner(_ context.Context, tenantID, userID, name, suggestionContext string) (models.Entity, error) {
	_ = suggestionContext
	f.nextID++
	entity := models.Entity{ID: "entity-" + itoa(f.nextID), TenantID: tenantID, Name: name}
	f.created = append(f.created, entity)
	return entity, nil
}

func (f *fakeSetupEntityStore) Update(_ context.Context, tenantID, entityID string, name *string, suggestionContext *string, fiscalYearStartMonth, fiscalYearStartDay *int) (models.Entity, error) {
	_ = tenantID
	_ = entityID
	_ = name
	_ = suggestionContext
	_ = fiscalYearStartMonth
	_ = fiscalYearStartDay
	return models.Entity{}, nil
}

func (f *fakeSetupEntityStore) Delete(_ context.Context, tenantID, entityID string) error {
	_ = tenantID
	_ = entityID
	return nil
}

func (f *fakeSetupEntityStore) GetRole(_ context.Context, tenantID, userID, entityID string) (models.Role, error) {
	_ = tenantID
	_ = userID
	_ = entityID
	return models.RoleAdmin, nil
}

type fakeSystemSettingsStore struct {
	setupComplete bool
	setCalls      int
}

func (f *fakeSystemSettingsStore) Get(_ context.Context) (db.SystemSettings, error) {
	return db.SystemSettings{SetupComplete: f.setupComplete}, nil
}

func (f *fakeSystemSettingsStore) SetSetupComplete(_ context.Context, _ time.Time) (db.SystemSettings, error) {
	f.setupComplete = true
	f.setCalls++
	return db.SystemSettings{SetupComplete: true}, nil
}

func (f *fakeSystemSettingsStore) UpsertSettings(_ context.Context, _ json.RawMessage) (db.SystemSettings, error) {
	return db.SystemSettings{SetupComplete: f.setupComplete}, nil
}

func TestSetupStatusRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		systemSettings: &fakeSystemSettingsStore{setupComplete: false},
	}
	r := gin.New()
	r.GET("/setup/status", hc.handleSetupStatus)

	req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Required {
		t.Fatal("expected required=true")
	}
}

func TestSetupStatusNotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		systemSettings: &fakeSystemSettingsStore{setupComplete: true},
	}
	r := gin.New()
	r.GET("/setup/status", hc.handleSetupStatus)

	req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Required {
		t.Fatal("expected required=false")
	}
}

func TestSetupCreatesAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &fakeUserStore{count: 0}
	tenants := &fakeTenantStore{count: 0}
	tenantMembers := &fakeTenantMembershipStore{}
	entities := &fakeSetupEntityStore{}
	systemSettings := &fakeSystemSettingsStore{setupComplete: false}
	hc := &HandlerContext{
		users:          users,
		tenants:        tenants,
		tenantMembers:  tenantMembers,
		entities:       entities,
		systemSettings: systemSettings,
	}
	r := gin.New()
	r.POST("/setup", hc.handleSetup)

	body := `{"tenant_name":"Acme","admin_email":"admin@test.local","admin_password":"s3cr3t!!","default_entity_name":"Acme LLC"}`
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var resp setupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TenantID == "" || resp.AdminUserID == "" {
		t.Fatal("expected tenant and admin ids")
	}
	if users.count != 1 || tenants.count != 1 {
		t.Fatalf("expected counts to increment, users=%d tenants=%d", users.count, tenants.count)
	}
	if len(tenantMembers.created) != 1 {
		t.Fatalf("expected tenant membership, got %d", len(tenantMembers.created))
	}
	if len(entities.created) != 1 {
		t.Fatalf("expected entity created, got %d", len(entities.created))
	}
	if systemSettings.setCalls != 1 {
		t.Fatalf("expected setup completion recorded")
	}
}

func TestSetupAlreadyComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := &HandlerContext{
		users:          &fakeUserStore{count: 1},
		tenants:        &fakeTenantStore{count: 1},
		tenantMembers:  &fakeTenantMembershipStore{},
		systemSettings: &fakeSystemSettingsStore{setupComplete: true},
	}
	r := gin.New()
	r.POST("/setup", hc.handleSetup)

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func itoa(val int) string {
	if val == 0 {
		return "0"
	}
	out := make([]byte, 0, 8)
	for val > 0 {
		out = append([]byte{byte('0' + val%10)}, out...)
		val /= 10
	}
	return string(out)
}
