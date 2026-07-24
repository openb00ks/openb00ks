package db

import (
	"context"
	"time"
)

// ScheduledTaskStore backs the ops-scheduler's recurring-task table. It is deliberately small: the
// task catalog + handlers live in code (internal/ops); this only persists schedule state and provides
// the multi-replica-safe claim.
type ScheduledTaskStore struct {
	db *DB
}

func NewScheduledTaskStore(db *DB) *ScheduledTaskStore {
	return &ScheduledTaskStore{db: db}
}

// Ensure registers a task with its default cadence, without clobbering operator-tuned rows. Called for
// every known task on scheduler startup.
func (s *ScheduledTaskStore) Ensure(ctx context.Context, name string, interval time.Duration, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (name, interval_seconds, enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO NOTHING
	`, name, int64(interval.Seconds()), enabled)
	return err
}

// ClaimDue atomically leases every enabled task that is due and not already running (or whose lease has
// expired), returning their names. FOR UPDATE SKIP LOCKED makes this safe across concurrent schedulers.
func (s *ScheduledTaskStore) ClaimDue(ctx context.Context, lease time.Duration) ([]string, error) {
	var names []string
	err := s.db.SelectContext(ctx, &names, `
		UPDATE scheduled_tasks
		SET running_since = now(), updated_at = now()
		WHERE name IN (
			SELECT name FROM scheduled_tasks
			WHERE enabled
			  AND next_run_at <= now()
			  AND (running_since IS NULL OR running_since < now() - make_interval(secs => $1))
			FOR UPDATE SKIP LOCKED
		)
		RETURNING name
	`, lease.Seconds())
	return names, err
}

// Complete records the outcome and schedules the next run (now + the row's current interval, so an
// operator change to interval_seconds takes effect immediately). Clears the lease.
func (s *ScheduledTaskStore) Complete(ctx context.Context, name, status, errMsg string, dur time.Duration) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET running_since    = NULL,
		    last_run_at      = now(),
		    last_status      = $2,
		    last_error       = NULLIF($3, ''),
		    last_duration_ms = $4,
		    next_run_at      = now() + make_interval(secs => interval_seconds),
		    updated_at       = now()
		WHERE name = $1
	`, name, status, errMsg, dur.Milliseconds())
	return err
}
