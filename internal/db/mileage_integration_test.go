//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestMileageStoreCreateUpdateDelete(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "mileage_logs")
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

	store := NewMileageStore(conn)
	log, err := store.Create(ctx, models.MileageLog{
		EntityID:      entityID,
		Date:          time.Now(),
		DistanceMiles: 12.5,
		StartLocation: "start",
		EndLocation:   "end",
		Purpose:       "test",
	})
	if err != nil {
		t.Fatalf("create mileage: %v", err)
	}

	updated, err := store.Update(ctx, log.ID, models.MileageLog{
		Date:          time.Now(),
		DistanceMiles: 20,
		StartLocation: "s2",
		EndLocation:   "e2",
		Purpose:       "p2",
	})
	if err != nil {
		t.Fatalf("update mileage: %v", err)
	}
	if updated.DistanceMiles != 20 {
		t.Fatalf("expected updated miles 20, got %v", updated.DistanceMiles)
	}

	if err := store.Delete(ctx, log.ID); err != nil {
		t.Fatalf("delete mileage: %v", err)
	}
	if _, err := store.GetByID(ctx, log.ID); err != ErrNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}
