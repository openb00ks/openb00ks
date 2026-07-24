//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestProcessingErrorStoreCreateList(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "processing_errors")
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

	store := NewProcessingErrorStore(conn)
	created, err := store.Create(ctx, models.ProcessingError{
		EntityID:  entityID,
		ReceiptID: receiptID,
		Stage:     "ocr",
		Error:     "boom",
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected id")
	}
	list, err := store.ListByReceiptID(ctx, receiptID, 10)
	if err != nil {
		t.Fatalf("list errors: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected errors")
	}
}
