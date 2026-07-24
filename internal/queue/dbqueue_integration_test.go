//go:build integration

package queue

import (
	"context"
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestDBQueueStageFilter(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entities")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "queue-test-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var receiptID string
	err = conn.GetContext(ctx, &receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "test-"+suffix, "test-"+suffix+".png")
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	q := NewDBQueue(conn)
	if _, err := q.Enqueue(ctx, EnqueueRequest{ReceiptID: receiptID, Stage: StageOCR}); err != nil {
		t.Fatalf("enqueue ocr: %v", err)
	}
	if _, err := q.Enqueue(ctx, EnqueueRequest{ReceiptID: receiptID, Stage: StageSuggest}); err != nil {
		t.Fatalf("enqueue suggest: %v", err)
	}

	jobs, err := q.Claim(ctx, ClaimRequest{WorkerID: "test", BatchSize: 5, Stage: StageSuggest})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Stage != StageSuggest {
		t.Fatalf("expected suggest stage, got %s", jobs[0].Stage)
	}
}

func TestDBQueueFailAndRequeue(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "receipt_jobs")
	ctx := context.Background()
	suffix := testutil.UniqueSuffix()

	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "queue-test-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM receipts WHERE entity_id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	var receiptID string
	if err := conn.GetContext(ctx, &receiptID, `
		INSERT INTO receipts (entity_id, storage_key, content_type, size_bytes, status, original_name)
		VALUES ($1, $2, 'image/png', 1, 'uploaded', $3)
		RETURNING id
	`, entityID, "test-"+suffix, "test-"+suffix+".png"); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	q := NewDBQueue(conn)
	job, err := q.Enqueue(ctx, EnqueueRequest{ReceiptID: receiptID, Stage: StageOCR})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := q.Fail(ctx, job.ID, "boom", true); err != nil {
		t.Fatalf("fail: %v", err)
	}

	var status string
	var lastError string
	if err := conn.QueryRowxContext(ctx, `SELECT status, COALESCE(last_error, '') FROM receipt_jobs WHERE id = $1`, job.ID).Scan(&status, &lastError); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "failed" || lastError != "boom" {
		t.Fatalf("expected failed+boom, got %s %s", status, lastError)
	}

	if err := q.Requeue(ctx, job.ID); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if err := conn.QueryRowxContext(ctx, `SELECT status, COALESCE(last_error, '') FROM receipt_jobs WHERE id = $1`, job.ID).Scan(&status, &lastError); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "queued" || lastError != "" {
		t.Fatalf("expected queued with empty error, got %s %s", status, lastError)
	}
}
