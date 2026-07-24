//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestVendorAliasStoreRecordAndList(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "vendor_aliases")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID) }()

	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	vendors := NewVendorStore(conn)
	vendor, err := vendors.Create(ctx, Vendor{EntityID: entityID, Name: "Blue Bottle Coffee", NormalizedName: "bluebottlecoffee"})
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	aliases := NewVendorAliasStore(conn)

	// First sighting of a raw string.
	if err := aliases.Record(ctx, vendor.ID, entityID, "SQ *BLUE BOTTLE #1", "sqbluebottle1"); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	// Same normalized string again → occurrences increments, no new row.
	if err := aliases.Record(ctx, vendor.ID, entityID, "SQ *BLUE BOTTLE #1", "sqbluebottle1"); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	// A distinct raw string → new alias.
	if err := aliases.Record(ctx, vendor.ID, entityID, "BLUEBOTTLE.COM", "bluebottlecom"); err != nil {
		t.Fatalf("record 3: %v", err)
	}
	// Blank normalized is a no-op (never records an empty alias).
	if err := aliases.Record(ctx, vendor.ID, entityID, "   ", ""); err != nil {
		t.Fatalf("record blank: %v", err)
	}

	normalized, err := aliases.ListNormalized(ctx, vendor.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(normalized) != 2 {
		t.Fatalf("expected 2 distinct aliases, got %d (%v)", len(normalized), normalized)
	}
	// Most-seen first: sqbluebottle1 has 2 occurrences.
	if normalized[0] != "sqbluebottle1" {
		t.Fatalf("expected the most-seen alias first, got %v", normalized)
	}

	// The vendor's counters reflect three recorded resolutions (blank excluded).
	refreshed, err := vendors.GetByID(ctx, vendor.ID)
	if err != nil {
		t.Fatalf("get vendor: %v", err)
	}
	if refreshed.ReceiptCount != 3 {
		t.Fatalf("expected receipt_count 3, got %d", refreshed.ReceiptCount)
	}
	if refreshed.LastSeen.IsZero() {
		t.Fatal("last_seen should be set after recording aliases")
	}
}
