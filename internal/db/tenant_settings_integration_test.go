//go:build integration

package db

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestTenantSettingsStoreGet_MissingReturnsEmpty(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "tenant_settings")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	store := NewTenantSettingsStore(conn)

	// Get should return empty settings for a tenant without settings
	settings, err := store.Get(ctx, tenantID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if settings.TenantID != tenantID {
		t.Errorf("expected tenant_id %s, got %s", tenantID, settings.TenantID)
	}
	if string(settings.SettingsJSON) != "{}" {
		t.Errorf("expected empty JSON {}, got %s", settings.SettingsJSON)
	}
}

func TestTenantSettingsStoreUpsert(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "tenant_settings")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenant_settings WHERE tenant_id = $1`, tenantID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	store := NewTenantSettingsStore(conn)

	// Insert new settings
	settingsJSON := json.RawMessage(`{"ai_provider":"openai"}`)
	settings, err := store.Upsert(ctx, tenantID, settingsJSON)
	if err != nil {
		t.Fatalf("Upsert (create): %v", err)
	}
	if settings.TenantID != tenantID {
		t.Errorf("expected tenant_id %s, got %s", tenantID, settings.TenantID)
	}
	if !jsonEqual(settings.SettingsJSON, settingsJSON) {
		t.Errorf("expected settings JSON, got %s", settings.SettingsJSON)
	}

	// Update existing settings
	updatedJSON := json.RawMessage(`{"ai_provider":"anthropic","usage_limit":1000}`)
	updated, err := store.Upsert(ctx, tenantID, updatedJSON)
	if err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	if !jsonEqual(updated.SettingsJSON, updatedJSON) {
		t.Errorf("expected updated settings JSON, got %s", updated.SettingsJSON)
	}

	// Verify Get returns updated settings
	fetched, err := store.Get(ctx, tenantID)
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if !jsonEqual(fetched.SettingsJSON, updatedJSON) {
		t.Errorf("expected fetched settings JSON, got %s", fetched.SettingsJSON)
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var left map[string]interface{}
	var right map[string]interface{}
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
