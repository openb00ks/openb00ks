//go:build integration

package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestEntityTaxSettingsStoreGetAndUpsert(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entity_tax_settings")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tax-tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "tax-entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
	}()

	store := NewEntityTaxSettingsStore(conn)

	empty, err := store.Get(ctx, entityID, 2026)
	if err != nil {
		t.Fatalf("get empty settings: %v", err)
	}
	if empty.EntityID != entityID || empty.TaxYear != 2026 {
		t.Fatalf("unexpected empty settings: %+v", empty)
	}

	updated, err := store.Upsert(
		ctx,
		entityID,
		2026,
		sql.NullInt64{Int64: 250, Valid: true},
		sql.NullInt64{Int64: 1000, Valid: true},
		sql.NullInt64{Int64: 75, Valid: true},
		sql.NullInt64{Int64: 60, Valid: true},
	)
	if err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if !updated.HomeOfficeSqFt.Valid || updated.HomeOfficeSqFt.Int64 != 250 {
		t.Fatalf("expected home office sqft to persist, got %+v", updated)
	}
	if percent, ok := UtilitiesBusinessUsePercent(updated.HomeOfficeSqFt, updated.HomeTotalSqFt); !ok || percent != 25 {
		t.Fatalf("expected utilities ratio 25, got %d ok=%v", percent, ok)
	}

	cleared, err := store.Upsert(
		ctx,
		entityID,
		2026,
		sql.NullInt64{},
		sql.NullInt64{},
		sql.NullInt64{Int64: 80, Valid: true},
		sql.NullInt64{},
	)
	if err != nil {
		t.Fatalf("clear settings: %v", err)
	}
	if cleared.HomeOfficeSqFt.Valid || cleared.HomeTotalSqFt.Valid {
		t.Fatalf("expected sqft values to clear, got %+v", cleared)
	}
	if !cleared.CellPhoneBusinessUsePercent.Valid || cleared.CellPhoneBusinessUsePercent.Int64 != 80 {
		t.Fatalf("expected cell phone percent to update, got %+v", cleared)
	}
}
