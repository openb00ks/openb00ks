package testutil

import (
	"context"
	"database/sql"
	"testing"
)

// RequireTable fails the test if the expected table is missing.
type DBGetter interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

func RequireTable(t *testing.T, conn DBGetter, table string) {
	t.Helper()
	var name sql.NullString
	if err := conn.GetContext(context.Background(), &name, `SELECT to_regclass($1)`, "public."+table); err != nil {
		t.Fatalf("schema check failed: %v", err)
	}
	if !name.Valid {
		t.Fatalf("schema not initialized; run migrations before integration tests")
	}
}
