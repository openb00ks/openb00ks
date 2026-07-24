// Package ops is a small, self-contained recurring-task scheduler for open-b00ks. Any background job
// that must run on a cadence — DB backups, notification digests, batch AI enrichment, cleanup —
// registers as a Task; the scheduler persists schedule state in the scheduled_tasks table and runs due
// tasks. It reuses the app's Postgres (no external cron/queue), so a self-hoster gets it for free.
package ops

import (
	"context"
	"log/slog"
	"time"
)

// Task is a registered recurring job. Run should be idempotent and honor ctx cancellation/deadline.
type Task struct {
	Name            string
	DefaultInterval time.Duration
	DefaultEnabled  bool
	// Timeout caps a single run; 0 means the scheduler's per-run default.
	Timeout time.Duration
	Run     func(ctx context.Context) error
}

// Store is the persistence the scheduler needs (satisfied by db.ScheduledTaskStore; faked in tests).
type Store interface {
	Ensure(ctx context.Context, name string, interval time.Duration, enabled bool) error
	ClaimDue(ctx context.Context, lease time.Duration) ([]string, error)
	Complete(ctx context.Context, name, status, errMsg string, dur time.Duration) error
}

// Metrics observes task runs; nil-safe (a no-op reporter is fine).
type Metrics interface {
	TaskRun(ctx context.Context, name, outcome string, dur time.Duration)
}

// Scheduler runs registered Tasks on their cadence.
type Scheduler struct {
	store        Store
	tasks        map[string]Task
	tick         time.Duration
	lease        time.Duration
	defaultRunTO time.Duration
	metrics      Metrics
	log          *slog.Logger
	now          func() time.Time // injectable for tests
}

// Options tune the scheduler. Zero values fall back to sane defaults.
type Options struct {
	Tick          time.Duration // how often to look for due tasks (default 30s)
	Lease         time.Duration // how long a claimed task is considered running before reclaim (default 1h)
	DefaultRunTO  time.Duration // per-run timeout when a Task sets none (default = Lease)
	Metrics       Metrics
	Log           *slog.Logger
	NowFuncForTst func() time.Time
}

func NewScheduler(store Store, opts Options) *Scheduler {
	s := &Scheduler{
		store:        store,
		tasks:        map[string]Task{},
		tick:         opts.Tick,
		lease:        opts.Lease,
		defaultRunTO: opts.DefaultRunTO,
		metrics:      opts.Metrics,
		log:          opts.Log,
		now:          opts.NowFuncForTst,
	}
	if s.tick <= 0 {
		s.tick = 30 * time.Second
	}
	if s.lease <= 0 {
		s.lease = time.Hour
	}
	if s.defaultRunTO <= 0 {
		s.defaultRunTO = s.lease
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// Register adds a task. Call before Run. A duplicate name overwrites (last registration wins).
func (s *Scheduler) Register(t Task) {
	if t.Name == "" || t.Run == nil {
		return
	}
	s.tasks[t.Name] = t
}

// Run seeds the catalog then loops until ctx is done, executing due tasks each tick. Blocks.
func (s *Scheduler) Run(ctx context.Context) error {
	for name, t := range s.tasks {
		if err := s.store.Ensure(ctx, name, t.DefaultInterval, t.DefaultEnabled); err != nil {
			return err
		}
	}
	s.log.Info("ops scheduler started", "tasks", len(s.tasks), "tick", s.tick.String())

	// Run an immediate pass so a just-due task doesn't wait a whole tick, then settle into the ticker.
	s.runDue(ctx)
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.runDue(ctx)
		}
	}
}

func (s *Scheduler) runDue(ctx context.Context) {
	names, err := s.store.ClaimDue(ctx, s.lease)
	if err != nil {
		s.log.Error("ops scheduler claim failed", "err", err)
		return
	}
	for _, name := range names {
		s.execute(ctx, name)
	}
}

func (s *Scheduler) execute(ctx context.Context, name string) {
	t, ok := s.tasks[name]
	if !ok {
		// A row with no registered handler (renamed/removed task). Mark it done so it doesn't spin.
		s.log.Warn("ops scheduler: no handler for task, skipping", "task", name)
		_ = s.store.Complete(ctx, name, "error", "no registered handler", 0)
		return
	}
	to := t.Timeout
	if to <= 0 {
		to = s.defaultRunTO
	}
	runCtx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	start := s.now()
	runErr := t.Run(runCtx)
	dur := s.now().Sub(start)

	status, msg := "success", ""
	if runErr != nil {
		status, msg = "error", runErr.Error()
		s.log.Error("ops task failed", "task", name, "err", runErr, "dur", dur.String())
	} else {
		s.log.Info("ops task ok", "task", name, "dur", dur.String())
	}
	if s.metrics != nil {
		s.metrics.TaskRun(ctx, name, status, dur)
	}
	// Use the parent ctx (not the possibly-timed-out runCtx) to record the result.
	if err := s.store.Complete(ctx, name, status, msg, dur); err != nil {
		s.log.Error("ops scheduler: failed to record completion", "task", name, "err", err)
	}
}
