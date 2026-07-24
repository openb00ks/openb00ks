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

func TestReportingStore(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "accounts")
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
		_, _ = conn.ExecContext(ctx, `DELETE FROM entries WHERE transaction_id IN (SELECT id FROM transactions WHERE entity_id = $1)`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM transactions WHERE entity_id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM accounts WHERE entity_id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
	}()

	var expenseID string
	if err := conn.GetContext(ctx, &expenseID, `INSERT INTO accounts (entity_id, name, type) VALUES ($1, $2, $3) RETURNING id`, entityID, "Expense", "expense"); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var cashID string
	if err := conn.GetContext(ctx, &cashID, `INSERT INTO accounts (entity_id, name, type) VALUES ($1, $2, $3) RETURNING id`, entityID, "Cash", "asset"); err != nil {
		t.Fatalf("insert account: %v", err)
	}

	trStore := NewTransactionStore(conn)
	date := time.Now()
	_, _, err = trStore.Create(ctx, entityID, date, "memo", "", []models.DraftEntry{
		{AccountID: expenseID, DebitCents: 100, CreditCents: 0},
		{AccountID: cashID, DebitCents: 0, CreditCents: 100},
	})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	reports := NewReportingStore(conn)
	rows, err := reports.GeneralLedger(ctx, entityID, date.Add(-time.Hour), date.Add(time.Hour))
	if err != nil {
		t.Fatalf("general ledger: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	trial, err := reports.TrialBalance(ctx, entityID, date.Add(-time.Hour), date.Add(time.Hour))
	if err != nil {
		t.Fatalf("trial balance: %v", err)
	}
	if len(trial) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(trial))
	}

	exportRows, err := reports.TransactionsForExport(ctx, entityID, date.Add(-time.Hour), date.Add(time.Hour))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(exportRows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(exportRows))
	}
}
