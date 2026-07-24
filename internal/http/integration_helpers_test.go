//go:build integration

package httpapi

import (
	"context"
	"testing"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func setupTenantUserEntity(t *testing.T, conn *db.DB) (tenantID, userID, entityID string, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	hash, _ := auth.HashPassword("testpass123")
	if err := conn.GetContext(ctx, &userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "user-"+suffix+"@test.local", hash); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE users SET default_tenant_id = $2 WHERE id = $1`, userID, tenantID); err != nil {
		t.Fatalf("set default tenant: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantID, userID); err != nil {
		t.Fatalf("insert tenant membership: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityID); err != nil {
		t.Fatalf("insert entity membership: %v", err)
	}

	cleanup = func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}
	return tenantID, userID, entityID, cleanup
}
