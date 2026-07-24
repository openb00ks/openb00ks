//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestReceiptArtifactsStores(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entities")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "artifacts-test-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var receiptID string
	err = conn.GetContext(ctx, &receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "test-"+suffix, "test-"+suffix+".png")
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	ocrStore := NewReceiptOCRStore(conn)
	if _, err := ocrStore.Create(ctx, models.ReceiptOCR{
		ReceiptID:  receiptID,
		Provider:   "test",
		Status:     "succeeded",
		DataJSON:   []byte(`{"total_cents":123}`),
		RunVersion: 1,
	}); err != nil {
		t.Fatalf("create ocr: %v", err)
	}
	ocrRows, err := ocrStore.ListByReceiptID(ctx, receiptID, 10)
	if err != nil {
		t.Fatalf("list ocr: %v", err)
	}
	if len(ocrRows) == 0 {
		t.Fatal("expected ocr row")
	}

	suggestStore := NewReceiptSuggestionStore(conn)
	if _, err := suggestStore.Create(ctx, models.ReceiptSuggestion{
		ReceiptID:  receiptID,
		Provider:   "test",
		Model:      "test-model",
		Status:     "succeeded",
		PromptJSON: []byte(`{"input":"test"}`),
		RawJSON:    []byte(`{"raw":"ok"}`),
		ParsedJSON: []byte(`{"account_id":"x"}`),
		Confidence: 0.5,
		RunVersion: 1,
	}); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	suggestionRows, err := suggestStore.ListByReceiptID(ctx, receiptID, 10)
	if err != nil {
		t.Fatalf("list suggestions: %v", err)
	}
	if len(suggestionRows) == 0 {
		t.Fatal("expected suggestion row")
	}
}
