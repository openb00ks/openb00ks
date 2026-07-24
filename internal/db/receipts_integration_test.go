//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestReceiptStoreCreateAndGet(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipts")
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

	store := NewReceiptStore(conn)
	receipt, err := store.Create(ctx, entityID, "key-"+suffix, "image/png", "uploaded", "receipt", "name.png", 10, 123)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if receipt.TotalCents != 123 {
		t.Fatalf("expected total 123, got %d", receipt.TotalCents)
	}
	if receipt.Kind != "receipt" {
		t.Fatalf("expected kind receipt, got %s", receipt.Kind)
	}

	entityCheck, err := store.GetEntityID(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("get entity id: %v", err)
	}
	if entityCheck != entityID {
		t.Fatalf("expected entity %s, got %s", entityID, entityCheck)
	}
}

func TestReceiptStoreListFilters(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipts")
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

	store := NewReceiptStore(conn)
	if _, err := store.Create(ctx, entityID, "key-a-"+suffix, "image/png", "uploaded", "receipt", "a.png", 10, 0); err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if _, err := store.Create(ctx, entityID, "key-b-"+suffix, "image/png", "processed", "import", "b.png", 10, 0); err != nil {
		t.Fatalf("create import: %v", err)
	}

	list, err := store.List(ctx, entityID, "uploaded", 10)
	if err != nil {
		t.Fatalf("list receipts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(list))
	}

	imports, err := store.ListByKind(ctx, entityID, "import", "", 10)
	if err != nil {
		t.Fatalf("list imports: %v", err)
	}
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
}
