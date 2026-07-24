//go:build integration

package db

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestAccountRoleTagsStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "account_role_tags")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "entity-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}

	store := NewAccountStore(conn, NewAccountRoleTagStore(conn))
	account, err := store.Create(ctx, entityID, "Utilities", "expense", "5030", "internet", "utilities")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if !reflect.DeepEqual(account.RoleTags, []string{"internet", "utilities"}) {
		t.Fatalf("create role tags = %#v", account.RoleTags)
	}
	if account.Code != "5030" {
		t.Fatalf("create code = %q, want 5030", account.Code)
	}

	loaded, err := store.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if !reflect.DeepEqual(loaded.RoleTags, []string{"internet", "utilities"}) {
		t.Fatalf("loaded role tags = %#v", loaded.RoleTags)
	}
	if loaded.Code != "5030" {
		t.Fatalf("loaded code = %q, want 5030", loaded.Code)
	}

	updated, err := store.Update(ctx, account.ID, "Utilities", "expense", "5031", "cell_phone")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	if !reflect.DeepEqual(updated.RoleTags, []string{"cell_phone"}) {
		t.Fatalf("updated role tags = %#v", updated.RoleTags)
	}
	if updated.Code != "5031" {
		t.Fatalf("updated code = %q, want 5031", updated.Code)
	}
}
