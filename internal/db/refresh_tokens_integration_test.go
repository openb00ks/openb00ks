//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestRefreshTokenStoreTenantID(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "refresh_tokens")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var userID string
	if err := conn.GetContext(ctx, &userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, "rt-"+suffix+"@test.local", "hash"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	store := NewRefreshTokenStore(conn)
	expires := time.Now().Add(time.Hour)
	tok, err := store.Create(ctx, userID, tenantID, "hash", expires)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if tok.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, tok.TenantID)
	}
}
