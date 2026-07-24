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
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestVendorRulesCRUD(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendor_rules")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	account, err := stores.Accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	createBody, _ := json.Marshal(map[string]string{
		"entity_id":  entityID,
		"match_type": "contains",
		"pattern":    "Starbucks",
		"account_id": account.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/vendor-rules", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	ruleID, _ := created["id"].(string)
	if ruleID == "" {
		t.Fatalf("missing rule id")
	}

	req = httptest.NewRequest(http.MethodGet, "/vendor-rules?entity_id="+entityID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	updateBody, _ := json.Marshal(map[string]string{
		"entity_id":  entityID,
		"match_type": "exact",
		"pattern":    "Starbucks",
		"account_id": account.ID,
	})
	req = httptest.NewRequest(http.MethodPatch, "/vendor-rules/"+ruleID, bytes.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/vendor-rules/"+ruleID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestVendorRulesUpdateDeleteUseRuleOwnershipForAuthorization(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendor_rules")

	_, _, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	otherTenantID, otherUserID, otherEntityID, otherCleanup := setupTenantUserEntity(t, conn)
	defer otherCleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	account, err := stores.Accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	otherAccount, err := stores.Accounts.Create(ctx, otherEntityID, "Travel", "expense", "")
	if err != nil {
		t.Fatalf("create other account: %v", err)
	}
	rule, err := stores.VendorRules.Create(ctx, dbRule(entityID, account.ID))
	if err != nil {
		t.Fatalf("create vendor rule: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(otherUserID, otherTenantID, time.Hour)

	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	updateBody, _ := json.Marshal(map[string]string{
		"entity_id":  otherEntityID,
		"match_type": "exact",
		"pattern":    "Forged",
		"account_id": otherAccount.ID,
	})
	req := httptest.NewRequest(http.MethodPatch, "/vendor-rules/"+rule.ID, bytes.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	stillRule, err := stores.VendorRules.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("get vendor rule: %v", err)
	}
	if stillRule.Pattern != "Starbucks" {
		t.Fatalf("expected rule unchanged, got pattern %q", stillRule.Pattern)
	}

	req = httptest.NewRequest(http.MethodDelete, "/vendor-rules/"+rule.ID+"?entity_id="+otherEntityID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	if _, err := stores.VendorRules.GetByID(ctx, rule.ID); err != nil {
		t.Fatalf("expected rule to remain after forbidden delete: %v", err)
	}
}

func dbRule(entityID, accountID string) models.VendorRule {
	return models.VendorRule{
		EntityID:  entityID,
		MatchType: "contains",
		Pattern:   "Starbucks",
		AccountID: accountID,
	}
}
