//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestReceiptAndMileageMetadataStores(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipt_metadata")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
	}()

	var receiptID string
	if err := conn.GetContext(ctx, &receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "test-"+suffix, "test-"+suffix+".png"); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	var mileageID string
	if err := conn.GetContext(ctx, &mileageID, `
		INSERT INTO mileage_logs (entity_id, date, distance_miles)
		VALUES ($1, CURRENT_DATE, 1.2)
		RETURNING id
	`, entityID); err != nil {
		t.Fatalf("insert mileage: %v", err)
	}

	receiptStore := NewReceiptMetadataStore(conn)
	if err := receiptStore.UpsertSuggestionContext(ctx, receiptID, "hello"); err != nil {
		t.Fatalf("upsert receipt: %v", err)
	}
	if got, err := receiptStore.GetSuggestionContext(ctx, receiptID); err != nil || got != "hello" {
		t.Fatalf("expected receipt context, got %q err=%v", got, err)
	}
	if err := receiptStore.UpsertSuggestionContext(ctx, receiptID, "updated"); err != nil {
		t.Fatalf("upsert receipt update: %v", err)
	}
	if got, _ := receiptStore.GetSuggestionContext(ctx, receiptID); got != "updated" {
		t.Fatalf("expected updated receipt context, got %q", got)
	}

	mileageStore := NewMileageMetadataStore(conn)
	if err := mileageStore.UpsertSuggestionContext(ctx, mileageID, "mctx"); err != nil {
		t.Fatalf("upsert mileage: %v", err)
	}
	if got, err := mileageStore.GetSuggestionContext(ctx, mileageID); err != nil || got != "mctx" {
		t.Fatalf("expected mileage context, got %q err=%v", got, err)
	}
}
