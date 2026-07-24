package db

import (
	"context"
	"time"

	"github.com/openb00ks/openb00ks/internal/aibatch"
)

// AIBatchStore is the Postgres implementation of aibatch.Store (ai_batch_jobs / ai_batch_items).
type AIBatchStore struct {
	db *DB
}

func NewAIBatchStore(db *DB) *AIBatchStore { return &AIBatchStore{db: db} }

var _ aibatch.Store = (*AIBatchStore)(nil)

// CreateJob records a submitted batch + its items atomically.
func (s *AIBatchStore) CreateJob(ctx context.Context, spec aibatch.JobSpec) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var jobID string
	if err := tx.QueryRowxContext(ctx, `
		INSERT INTO ai_batch_jobs (kind, provider, model, provider_batch_id, item_count)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, spec.Kind, spec.Provider, spec.Model, spec.ProviderBatchID, len(spec.Items)).Scan(&jobID); err != nil {
		return err
	}
	for _, it := range spec.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ai_batch_items (batch_job_id, custom_id, ref_id) VALUES ($1, $2, $3)
		`, jobID, it.CustomID, it.RefID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type aiBatchJobRow struct {
	ID              string    `db:"id"`
	Kind            string    `db:"kind"`
	ProviderBatchID string    `db:"provider_batch_id"`
	Status          string    `db:"status"`
	SubmittedAt     time.Time `db:"submitted_at"`
}

func (s *AIBatchStore) OpenJobs(ctx context.Context) ([]aibatch.Job, error) {
	return s.queryJobs(ctx, `
		SELECT id::text AS id, kind, provider_batch_id, status, submitted_at
		FROM ai_batch_jobs WHERE status = 'submitted' ORDER BY submitted_at`)
}

func (s *AIBatchStore) StuckJobs(ctx context.Context, olderThan time.Time) ([]aibatch.Job, error) {
	return s.queryJobs(ctx, `
		SELECT id::text AS id, kind, provider_batch_id, status, submitted_at
		FROM ai_batch_jobs WHERE status = 'submitted' AND submitted_at < $1 ORDER BY submitted_at`, olderThan)
}

func (s *AIBatchStore) queryJobs(ctx context.Context, query string, args ...interface{}) ([]aibatch.Job, error) {
	rows := []aiBatchJobRow{}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	jobs := make([]aibatch.Job, 0, len(rows))
	for _, r := range rows {
		jobs = append(jobs, aibatch.Job{
			ID: r.ID, Kind: r.Kind, ProviderBatchID: r.ProviderBatchID, Status: r.Status, SubmittedAt: r.SubmittedAt,
		})
	}
	return jobs, nil
}

type aiBatchItemRow struct {
	CustomID string `db:"custom_id"`
	RefID    string `db:"ref_id"`
}

func (s *AIBatchStore) Items(ctx context.Context, jobID string) ([]aibatch.JobItem, error) {
	rows := []aiBatchItemRow{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT custom_id, ref_id FROM ai_batch_items WHERE batch_job_id = $1`, jobID); err != nil {
		return nil, err
	}
	items := make([]aibatch.JobItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, aibatch.JobItem{CustomID: r.CustomID, RefID: r.RefID})
	}
	return items, nil
}

func (s *AIBatchStore) MarkItem(ctx context.Context, jobID, customID, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE ai_batch_items SET status = $3, error = NULLIF($4, ''), updated_at = now()
		WHERE batch_job_id = $1 AND custom_id = $2`, jobID, customID, status, errMsg)
	return err
}

func (s *AIBatchStore) MarkJob(ctx context.Context, jobID, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE ai_batch_jobs
		SET status = $2,
		    last_error = NULLIF($3, ''),
		    completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END,
		    updated_at = now()
		WHERE id = $1`, jobID, status, errMsg)
	return err
}
