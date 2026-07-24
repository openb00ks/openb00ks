//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestVendorsCRUD(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendors")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	meals, err := stores.Accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	software, err := stores.Accounts.Create(ctx, entityID, "Software", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	capture := &captureVendorProvider{}
	hc.SetSearchProvider(capture)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	do := func(method, path, tok string, body any) *httptest.ResponseRecorder {
		var r *http.Request
		if body != nil {
			b, _ := json.Marshal(body)
			r = httptest.NewRequest(method, path, bytes.NewReader(b))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}

	// Create: name required; normalized_name is derived server-side; default account set.
	w := do(http.MethodPost, "/vendors", token, map[string]string{
		"entity_id":          entityID,
		"name":               "Blue Bottle Coffee",
		"match_pattern":      "BLUE BOTTLE",
		"default_account_id": meals.ID,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (%s)", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	vendorID, _ := created["id"].(string)
	if vendorID == "" {
		t.Fatalf("missing vendor id: %v", created)
	}
	if created["normalized_name"] != "bluebottlecoffee" {
		t.Fatalf("normalized_name should be derived, got %v", created["normalized_name"])
	}
	if created["default_account_id"] != meals.ID {
		t.Fatalf("default_account_id round-trip failed: %v", created["default_account_id"])
	}

	// List: the vendor shows up for its entity.
	w = do(http.MethodGet, "/vendors?entity_id="+entityID, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var list struct {
		Rows []map[string]any `json:"rows"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Rows) != 1 || list.Rows[0]["id"] != vendorID {
		t.Fatalf("list should contain the created vendor, got %v", list.Rows)
	}

	// Get by id.
	w = do(http.MethodGet, "/vendors/"+vendorID, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Update: rename + move the default account; fields overwrite (not merge).
	w = do(http.MethodPatch, "/vendors/"+vendorID, token, map[string]string{
		"name":               "Blue Bottle",
		"match_pattern":      "BLUEBOTTLE",
		"default_account_id": software.ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["name"] != "Blue Bottle" || updated["default_account_id"] != software.ID {
		t.Fatalf("update didn't overwrite fields: %v", updated)
	}

	// The retrieval index (_vendors) must stay fresh across the edit — retrieval reads the default
	// account straight from this doc, so a stale one would misfile future receipts.
	last := capture.lastVendorUpsert()
	if last == nil || last.ID != vendorID || last.DefaultAccountID != software.ID {
		t.Fatalf("update must refresh the _vendors doc with the new default account, got %+v", last)
	}

	// Delete, then confirm it's gone.
	w = do(http.MethodDelete, "/vendors/"+vendorID, token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}
	if !capture.deletedVendor(vendorID) {
		t.Fatalf("delete must remove the vendor from the _vendors index")
	}
	w = do(http.MethodGet, "/vendors/"+vendorID, token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", w.Code)
	}
}

// captureVendorProvider records _vendors index writes (Noop for everything else) so the test can assert
// the API keeps the retrieval document in sync.
type captureVendorProvider struct {
	searchpkg.NoopProvider
	upserts []searchpkg.VendorDocument
	deletes []string
}

func (p *captureVendorProvider) UpsertVendor(_ context.Context, doc searchpkg.VendorDocument) error {
	p.upserts = append(p.upserts, doc)
	return nil
}

func (p *captureVendorProvider) DeleteVendor(_ context.Context, id string) error {
	p.deletes = append(p.deletes, id)
	return nil
}

func (p *captureVendorProvider) lastVendorUpsert() *searchpkg.VendorDocument {
	if len(p.upserts) == 0 {
		return nil
	}
	return &p.upserts[len(p.upserts)-1]
}

func (p *captureVendorProvider) deletedVendor(id string) bool {
	for _, d := range p.deletes {
		if d == id {
			return true
		}
	}
	return false
}

func TestVendorsAuthorizationUsesVendorOwnership(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendors")

	_, _, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	otherTenantID, otherUserID, _, otherCleanup := setupTenantUserEntity(t, conn)
	defer otherCleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	acct, err := stores.Accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	vendor, err := stores.Vendors.Create(ctx, db.Vendor{
		EntityID: entityID, Name: "Acme", NormalizedName: "acme", DefaultAccountID: acct.ID,
	})
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	otherToken, _ := tokens.Issue(otherUserID, otherTenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	// A user from another tenant may not update this vendor — authz resolves the entity from the
	// vendor's own ownership, not from the request body.
	body, _ := json.Marshal(map[string]string{"name": "Forged"})
	req := httptest.NewRequest(http.MethodPatch, "/vendors/"+vendor.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+otherToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant update: expected 403, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/vendors/"+vendor.ID, nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant delete: expected 403, got %d", w.Code)
	}

	// The vendor survives both forbidden calls.
	if _, err := stores.Vendors.GetByID(ctx, vendor.ID); err != nil {
		t.Fatalf("vendor should remain after forbidden ops: %v", err)
	}
}
