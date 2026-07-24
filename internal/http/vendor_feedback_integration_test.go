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

func aliasesContain(aliases []string, want string) bool {
	for _, a := range aliases {
		if a == want {
			return true
		}
	}
	return false
}

// TestReviewerFeedbackLoop_PostLearnsCorrectedAccount is the end-to-end proof of the feedback loop:
// a vendor's default account is one thing, the reviewer posts the receipt to a DIFFERENT expense account,
// and after posting the vendor's default account has become the reviewer's choice + the raw string is
// recorded as an alias.
func TestReviewerFeedbackLoop_PostLearnsCorrectedAccount(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendors")
	testutil.RequireTable(t, conn, "vendor_aliases")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()

	// Two expense accounts (the AI's guess vs. the reviewer's choice) + a cash account for the credit leg.
	meals, err := stores.Accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create meals: %v", err)
	}
	software, err := stores.Accounts.Create(ctx, entityID, "Software", "expense", "")
	if err != nil {
		t.Fatalf("create software: %v", err)
	}
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}

	// A vendor the pipeline "resolved" with Meals as its default account (the AI's classification).
	vendor, err := stores.Vendors.Create(ctx, db.Vendor{
		EntityID: entityID, Name: "Acme", NormalizedName: "acme", DefaultAccountID: meals.ID,
	})
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	// A receipt linked to that vendor (as the pipeline would persist it) with a draft the reviewer has
	// categorized to Software — overruling the AI's Meals.
	receipt, err := stores.Receipts.Create(ctx, entityID, "key-1", "image/png", "ready_for_review", "receipt", "acme.png", 100, 1080)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if err := stores.Receipts.SetResolvedVendor(ctx, receipt.ID, vendor.ID, "SQ *ACME #42"); err != nil {
		t.Fatalf("set resolved vendor: %v", err)
	}
	if _, err := stores.Drafts.EnsureForReceipt(ctx, receipt.ID); err != nil {
		t.Fatalf("ensure draft: %v", err)
	}
	if _, err := stores.Drafts.UpdateDraft(ctx, receipt.ID, time.Now().UTC(), "Acme", []models.DraftEntry{
		{AccountID: software.ID, DebitCents: 1080},
		{AccountID: cash.ID, CreditCents: 1080},
	}); err != nil {
		t.Fatalf("update draft: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	// Post the receipt's draft.
	req := httptest.NewRequest(http.MethodPost, "/receipts/"+receipt.ID+"/post", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("post: expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	// The vendor learned the reviewer's correction: default account is now Software, not Meals.
	learned, err := stores.Vendors.GetByID(ctx, vendor.ID)
	if err != nil {
		t.Fatalf("get vendor: %v", err)
	}
	if learned.DefaultAccountID != software.ID {
		t.Fatalf("vendor default account should have learned Software (%s), got %q", software.ID, learned.DefaultAccountID)
	}

	// The raw receipt string was reinforced as an alias.
	aliases, err := stores.VendorAliases.ListNormalized(ctx, vendor.ID)
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}
	found := false
	for _, a := range aliases {
		if a == "sqacme42" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the posted raw string recorded as an alias, got %v", aliases)
	}
}

// TestVendorCorrection_ReassignsAliasAndLearnsCorrectVendor proves the vendor-correction path: the
// pipeline mis-matched vendor A (and recorded A's alias), the reviewer re-points the receipt at vendor B
// and posts to Software — after which B (not A) owns the alias and has learned the account.
func TestVendorCorrection_ReassignsAliasAndLearnsCorrectVendor(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendor_aliases")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()
	stores := db.NewStores(conn)
	ctx := context.Background()

	software, err := stores.Accounts.Create(ctx, entityID, "Software", "expense", "")
	if err != nil {
		t.Fatalf("create software: %v", err)
	}
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}

	// The AI's wrong guess (A) vs. the reviewer's correction (B).
	vendorA, err := stores.Vendors.Create(ctx, db.Vendor{EntityID: entityID, Name: "Acme Wrong", NormalizedName: "acmewrong"})
	if err != nil {
		t.Fatalf("create vendor A: %v", err)
	}
	vendorB, err := stores.Vendors.Create(ctx, db.Vendor{EntityID: entityID, Name: "Acme Right", NormalizedName: "acmeright"})
	if err != nil {
		t.Fatalf("create vendor B: %v", err)
	}

	receipt, err := stores.Receipts.Create(ctx, entityID, "k-2", "image/png", "ready_for_review", "receipt", "acme.png", 100, 1200)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	// The pipeline resolved (wrongly) to A and recorded A's alias.
	if err := stores.Receipts.SetResolvedVendor(ctx, receipt.ID, vendorA.ID, "SQ *ACME #7"); err != nil {
		t.Fatalf("set resolved vendor: %v", err)
	}
	if err := stores.VendorAliases.Record(ctx, vendorA.ID, entityID, "SQ *ACME #7", "sqacme7"); err != nil {
		t.Fatalf("seed alias on A: %v", err)
	}
	if _, err := stores.Drafts.EnsureForReceipt(ctx, receipt.ID); err != nil {
		t.Fatalf("ensure draft: %v", err)
	}
	if _, err := stores.Drafts.UpdateDraft(ctx, receipt.ID, time.Now().UTC(), "Acme", []models.DraftEntry{
		{AccountID: software.ID, DebitCents: 1200},
		{AccountID: cash.ID, CreditCents: 1200},
	}); err != nil {
		t.Fatalf("update draft: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	// Reviewer corrects the vendor to B.
	body, _ := json.Marshal(map[string]string{"vendor_id": vendorB.ID})
	req := httptest.NewRequest(http.MethodPatch, "/receipts/"+receipt.ID+"/vendor", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("vendor correction: expected 204, got %d (%s)", w.Code, w.Body.String())
	}

	// Then posts.
	req = httptest.NewRequest(http.MethodPost, "/receipts/"+receipt.ID+"/post", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("post: expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	// B learned the account.
	learned, err := stores.Vendors.GetByID(ctx, vendorB.ID)
	if err != nil {
		t.Fatalf("get vendor B: %v", err)
	}
	if learned.DefaultAccountID != software.ID {
		t.Fatalf("corrected vendor B should have learned Software, got %q", learned.DefaultAccountID)
	}

	// The alias moved from A to B.
	aliasesB, _ := stores.VendorAliases.ListNormalized(ctx, vendorB.ID)
	if !aliasesContain(aliasesB, "sqacme7") {
		t.Fatalf("alias should have been reassigned to B, got %v", aliasesB)
	}
	aliasesA, _ := stores.VendorAliases.ListNormalized(ctx, vendorA.ID)
	if aliasesContain(aliasesA, "sqacme7") {
		t.Fatalf("alias should no longer belong to the wrong vendor A, got %v", aliasesA)
	}
}
