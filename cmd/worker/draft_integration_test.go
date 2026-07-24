//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestRunSuggestAndDraftForImportCreatesBalancedDraft(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}

	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()
	testutil.RequireTable(t, conn, "receipts")
	testutil.RequireTable(t, conn, "receipt_suggestions")
	testutil.RequireTable(t, conn, "draft_entries")
	testutil.RequireTable(t, conn, "accounts")
	testutil.RequireTable(t, conn, "vendor_rules")

	suffix := testutil.UniqueSuffix()
	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "worker-import-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	receipts := db.NewReceiptStore(conn)
	drafts := db.NewDraftStore(conn)
	accounts := db.NewAccountStore(conn)
	rules := db.NewVendorRuleStore(conn)
	ocr := db.NewReceiptOCRStore(conn)
	suggestions := db.NewReceiptSuggestionStore(conn)

	cashAccount, err := accounts.Create(ctx, entityID, "Cash", "asset", "")
	if err != nil {
		t.Fatalf("create cash account: %v", err)
	}
	expenseAccount, err := accounts.Create(ctx, entityID, "Meals", "expense", "")
	if err != nil {
		t.Fatalf("create expense account: %v", err)
	}
	_, err = rules.Create(ctx, models.VendorRule{
		EntityID:  entityID,
		MatchType: "contains",
		Pattern:   "coffee shop",
		AccountID: expenseAccount.ID,
	})
	if err != nil {
		t.Fatalf("create vendor rule: %v", err)
	}

	receipt, err := receipts.Create(
		ctx,
		entityID,
		"import-"+suffix,
		"text/csv",
		"uploaded",
		"import",
		"import-"+suffix+".csv",
		1,
		0,
	)
	if err != nil {
		t.Fatalf("create import receipt: %v", err)
	}

	_, err = ocr.Create(ctx, models.ReceiptOCR{
		ReceiptID: receipt.ID,
		Provider:  "import",
		Status:    "succeeded",
		RawText: "date,vendor,amount\n" +
			"2026-05-01,Coffee Shop Midtown,-12.00\n" +
			"2026-05-02,Coffee Shop Downtown,-8.00\n",
		DataJSON:   []byte(`{}`),
		RunVersion: 1,
	})
	if err != nil {
		t.Fatalf("create ocr row: %v", err)
	}

	w := &worker{
		receipts:    receipts,
		drafts:      drafts,
		rules:       rules,
		ocr:         ocr,
		suggestions: suggestions,
		accounts:    accounts,
	}
	if err := w.runSuggest(ctx, receipt.ID); err != nil {
		t.Fatalf("run suggest: %v", err)
	}
	if err := w.runDraft(ctx, receipt.ID); err != nil {
		t.Fatalf("run draft: %v", err)
	}

	latestSuggestion, err := suggestions.LatestByReceiptID(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("latest suggestion: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(latestSuggestion.ParsedJSON, &parsed); err != nil {
		t.Fatalf("unmarshal parsed json: %v", err)
	}
	rows, ok := parsed["import_rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("expected import_rows in suggestion, got: %v", parsed["import_rows"])
	}

	draft, err := drafts.GetByReceiptID(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if len(draft.Entries) != 2 {
		t.Fatalf("expected 2 draft entries, got %d (%+v)", len(draft.Entries), draft.Entries)
	}

	debitsByAccount := map[string]int64{}
	creditsByAccount := map[string]int64{}
	for _, entry := range draft.Entries {
		debitsByAccount[entry.AccountID] += entry.DebitCents
		creditsByAccount[entry.AccountID] += entry.CreditCents
	}
	if debitsByAccount[expenseAccount.ID] != 2000 {
		t.Fatalf("expected expense debits=2000, got %d (entries=%+v)", debitsByAccount[expenseAccount.ID], draft.Entries)
	}
	if creditsByAccount[cashAccount.ID] != 2000 {
		t.Fatalf("expected cash credits=2000, got %d (entries=%+v)", creditsByAccount[cashAccount.ID], draft.Entries)
	}

	updatedReceipt, err := receipts.GetByID(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("get receipt: %v", err)
	}
	if updatedReceipt.Status != "ready_for_review" {
		t.Fatalf("expected ready_for_review, got %s", updatedReceipt.Status)
	}
}
