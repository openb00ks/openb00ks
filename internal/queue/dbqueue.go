package queue

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/openb00ks/openb00ks/internal/db"
)

type DBQueue struct {
	db *db.DB
}

func NewDBQueue(dbConn *db.DB) *DBQueue {
	return &DBQueue{db: dbConn}
}

func (q *DBQueue) Enqueue(ctx context.Context, req EnqueueRequest) (Job, error) {
	stage := req.Stage
	if stage == "" {
		stage = StageOCR
	}
	row := jobRow{}
	err := q.db.QueryRowxContext(ctx, `
		INSERT INTO receipt_jobs (receipt_id, stage, status)
		VALUES ($1, $2, 'queued')
		RETURNING id, receipt_id, stage, status, attempts, max_attempts, locked_by, last_error
	`, req.ReceiptID, stage).StructScan(&row)
	if err != nil {
		return Job{}, err
	}
	return jobFromRow(row), nil
}

func (q *DBQueue) Claim(ctx context.Context, req ClaimRequest) ([]Job, error) {
	if req.BatchSize <= 0 {
		req.BatchSize = 10
	}
	if req.LockSeconds <= 0 {
		req.LockSeconds = 300
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 5
	}

	rows := []jobRow{}
	stageFilter := ""
	args := []interface{}{req.MaxAttempts, req.BatchSize, req.LockSeconds, req.WorkerID}
	if req.Stage != "" {
		stageFilter = " AND stage = $5"
		args = append(args, req.Stage)
	}
	err := q.db.SelectContext(ctx, &rows, `
		WITH cte AS (
			SELECT id
			FROM receipt_jobs
			WHERE status IN ('queued', 'failed')
			  AND attempts < $1
			  AND (locked_until IS NULL OR locked_until < now())
			  `+stageFilter+`
			ORDER BY created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE receipt_jobs
		SET status = 'processing',
		    attempts = attempts + 1,
		    locked_until = now() + ($3 * interval '1 second'),
		    locked_by = $4,
		    updated_at = now()
		WHERE id IN (SELECT id FROM cte)
		RETURNING id, receipt_id, stage, status, attempts, max_attempts, locked_by, last_error
	`, args...)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, jobFromRow(row))
	}
	return jobs, nil
}

func (q *DBQueue) Ack(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE receipt_jobs
		SET status = 'done', locked_until = NULL, locked_by = NULL, updated_at = now()
		WHERE id = $1
	`, jobID)
	return err
}

func (q *DBQueue) Fail(ctx context.Context, jobID string, errMsg string, retry bool) error {
	status := "failed"
	if !retry {
		status = "dead"
	}
	_, err := q.db.ExecContext(ctx, `
		UPDATE receipt_jobs
		SET status = $2, locked_until = NULL, locked_by = NULL, last_error = $3, updated_at = now()
		WHERE id = $1
	`, jobID, status, errMsg)
	return err
}

func (q *DBQueue) Requeue(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE receipt_jobs
		SET status = 'queued', locked_until = NULL, locked_by = NULL, last_error = NULL, updated_at = now()
		WHERE id = $1
	`, jobID)
	return err
}

type jobRow struct {
	ID          string         `db:"id"`
	ReceiptID   string         `db:"receipt_id"`
	Status      string         `db:"status"`
	Attempts    int            `db:"attempts"`
	MaxAttempts int            `db:"max_attempts"`
	Stage       string         `db:"stage"`
	LockedBy    sql.NullString `db:"locked_by"`
	LastError   sql.NullString `db:"last_error"`
}

func jobFromRow(row jobRow) Job {
	job := Job{
		ID:          row.ID,
		ReceiptID:   row.ReceiptID,
		Stage:       JobStage(row.Stage),
		Status:      JobStatus(row.Status),
		Attempts:    row.Attempts,
		MaxAttempts: row.MaxAttempts,
	}
	if row.LockedBy.Valid {
		job.LockedBy = row.LockedBy.String
	}
	if row.LastError.Valid {
		job.LastError = row.LastError.String
	}
	return job
}

var ErrQueueUnavailable = errors.New("queue unavailable")

func (q *DBQueue) Ready(ctx context.Context) error {
	if q == nil || q.db == nil {
		return ErrQueueUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return q.db.PingContext(ctx)
}
