//go:build integration

package httpapi

import (
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

func TestAccountBalancesLedgerAndDeleteGuard(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entries")

	tenantID, userID, entityID, cleanup := setupTenantUserEntity(t, conn)
	defer cleanup()

	stores := db.NewStores(conn)
	ctx := context.Background()
	cash, err := stores.Accounts.Create(ctx, entityID, "Cash", "asset", "1000")
	if err != nil {
		t.Fatalf("create cash: %v", err)
	}
	revenue, err := stores.Accounts.Create(ctx, entityID, "Revenue", "income", "4000")
	if err != nil {
		t.Fatalf("create revenue: %v", err)
	}
	unused, err := stores.Accounts.Create(ctx, entityID, "Unused", "expense", "5000")
	if err != nil {
		t.Fatalf("create unused: %v", err)
	}
	trDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if _, _, err := stores.Transactions.Create(ctx, entityID, trDate, "sale", "", []models.DraftEntry{
		{AccountID: cash.ID, DebitCents: 10000},
		{AccountID: revenue.ID, CreditCents: 10000},
	}); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)
	objects := storage.NewLocalStore(t.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	hc.SetStores(stores, nil)
	server := NewServer(hc)
	gin.SetMode(gin.TestMode)

	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, req)
		return w
	}

	// Balances: cash (asset, debit) and revenue (income, credit) both net to +10000; unused is 0.
	w := do(http.MethodGet, "/entities/"+entityID+"/account-balances")
	if w.Code != http.StatusOK {
		t.Fatalf("balances status = %d: %s", w.Code, w.Body.String())
	}
	var balResp struct {
		Balances []struct {
			AccountID    string `json:"account_id"`
			BalanceCents int64  `json:"balance_cents"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &balResp); err != nil {
		t.Fatalf("decode balances: %v", err)
	}
	got := map[string]int64{}
	for _, b := range balResp.Balances {
		got[b.AccountID] = b.BalanceCents
	}
	if got[cash.ID] != 10000 {
		t.Errorf("cash balance = %d, want 10000", got[cash.ID])
	}
	if got[revenue.ID] != 10000 {
		t.Errorf("revenue balance = %d, want 10000", got[revenue.ID])
	}
	if got[unused.ID] != 0 {
		t.Errorf("unused balance = %d, want 0", got[unused.ID])
	}

	// Account transactions: one row + balance.
	w = do(http.MethodGet, "/accounts/"+cash.ID+"/transactions")
	if w.Code != http.StatusOK {
		t.Fatalf("account transactions status = %d: %s", w.Code, w.Body.String())
	}
	var txResp struct {
		Account      struct{ ID, Name, Code string } `json:"account"`
		BalanceCents int64                           `json:"balance_cents"`
		Rows         []struct {
			DebitCents int64 `json:"debit_cents"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &txResp); err != nil {
		t.Fatalf("decode account transactions: %v", err)
	}
	if txResp.Account.Code != "1000" || txResp.BalanceCents != 10000 || len(txResp.Rows) != 1 || txResp.Rows[0].DebitCents != 10000 {
		t.Fatalf("unexpected account transactions: %+v", txResp)
	}

	// Delete guard: cash has entries → 409; unused has none → 204.
	if w := do(http.MethodDelete, "/accounts/"+cash.ID); w.Code != http.StatusConflict {
		t.Fatalf("delete in-use account status = %d, want 409: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodDelete, "/accounts/"+unused.ID); w.Code != http.StatusNoContent {
		t.Fatalf("delete unused account status = %d, want 204: %s", w.Code, w.Body.String())
	}
}
