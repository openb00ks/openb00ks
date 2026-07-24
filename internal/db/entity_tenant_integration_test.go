//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestEntityStoreListForUserTenantScope(t *testing.T) {
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

	var userID string
	if err := conn.GetContext(ctx, &userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "tenant-scope-"+suffix+"@test.local", "hash"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	var tenantA string
	if err := conn.GetContext(ctx, &tenantA, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-a-"+suffix); err != nil {
		t.Fatalf("insert tenant a: %v", err)
	}
	var tenantB string
	if err := conn.GetContext(ctx, &tenantB, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-b-"+suffix); err != nil {
		t.Fatalf("insert tenant b: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1 OR id = $2`, tenantA, tenantB)
	}()

	var entityA string
	if err := conn.GetContext(ctx, &entityA, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantA, "entity-a-"+suffix); err != nil {
		t.Fatalf("insert entity a: %v", err)
	}
	var entityB string
	if err := conn.GetContext(ctx, &entityB, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantB, "entity-b-"+suffix); err != nil {
		t.Fatalf("insert entity b: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entity_users WHERE entity_id = $1 OR entity_id = $2`, entityA, entityB)
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1 OR id = $2`, entityA, entityB)
	}()

	if _, err := conn.ExecContext(ctx, `INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityA); err != nil {
		t.Fatalf("insert membership a: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityB); err != nil {
		t.Fatalf("insert membership b: %v", err)
	}

	store := NewEntityStore(conn)
	entities, err := store.ListForUser(ctx, tenantA, userID, 10)
	if err != nil {
		t.Fatalf("list entities: %v", err)
	}
	if len(entities) != 1 || entities[0].ID != entityA {
		t.Fatalf("expected only entityA, got %+v", entities)
	}
}

func TestTenantMembershipStoreListForUser(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "tenant_memberships")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var userID string
	if err := conn.GetContext(ctx, &userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "tm-user-"+suffix+"@test.local", "hash"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	var tenantA string
	if err := conn.GetContext(ctx, &tenantA, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-a-"+suffix); err != nil {
		t.Fatalf("insert tenant a: %v", err)
	}
	var tenantB string
	if err := conn.GetContext(ctx, &tenantB, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-b-"+suffix); err != nil {
		t.Fatalf("insert tenant b: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1 OR id = $2`, tenantA, tenantB)
	}()

	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantA, userID); err != nil {
		t.Fatalf("insert membership a: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'user')`, tenantB, userID); err != nil {
		t.Fatalf("insert membership b: %v", err)
	}

	store := NewTenantMembershipStore(conn)
	memberships, err := store.ListForUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships, got %d", len(memberships))
	}
}
