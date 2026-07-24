// Package aibatch is a general asynchronous AI batch framework. Any batch AI operation — the receipt
// pipeline stages, and future needs — registers a Kind (Gather pending work → Apply results → Reset on
// failure); one generic Submit/Collect pair drives them all through a provider's Batch API (~50%
// cheaper, latency-tolerant). Generalizes per-stage hardcoded tasks: the "kind" is an opaque
// string (no schema/task change to add an operation) and ref ids are generic (any domain entity).
//
// Persistence is behind the Store interface (db.AIBatchStore in prod, a fake in tests), so the
// submit/collect logic is unit-testable without a database.
package aibatch

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/spectrum-labs-tech/ai"
)

// Item is one unit of pending work: a provider request tied to a domain entity. The framework sets
// Request.CustomID.
type Item struct {
	RefID   string
	Request ai.BatchRequest
}

// Kind is a registered batch AI operation.
type Kind interface {
	// Name is the stable identifier stored on jobs (e.g. "receipt-extract").
	Name() string
	// Gather claims a bounded set of pending entities (marking them in-flight in the domain) and
	// returns their requests. An empty slice means nothing to submit this round.
	Gather(ctx context.Context) ([]Item, error)
	// Apply handles one completed result for an entity (parse → validate → advance). resultErr is set
	// when the provider reported a per-item error.
	Apply(ctx context.Context, refID, result string, resultErr error) error
	// Reset returns entities to pending (a batch failed/expired, or never submitted) so a later round
	// retries them.
	Reset(ctx context.Context, refIDs []string) error
}

// Registry maps kind name → Kind.
type Registry struct{ kinds map[string]Kind }

func NewRegistry() *Registry { return &Registry{kinds: map[string]Kind{}} }

func (r *Registry) Register(k Kind) { r.kinds[k.Name()] = k }

func (r *Registry) Get(name string) (Kind, bool) { k, ok := r.kinds[name]; return k, ok }

func (r *Registry) All() []Kind {
	out := make([]Kind, 0, len(r.kinds))
	for _, k := range r.kinds {
		out = append(out, k)
	}
	return out
}

// Job is an in-flight (or terminal) batch job.
type Job struct {
	ID              string
	Kind            string
	ProviderBatchID string
	Status          string // submitted | completed | failed | expired
	SubmittedAt     time.Time
}

// JobItem links a batch item's custom_id to its domain entity.
type JobItem struct {
	CustomID string
	RefID    string
}

// JobSpec is a submitted batch to persist.
type JobSpec struct {
	Kind            string
	Provider        string
	Model           string
	ProviderBatchID string
	Items           []JobItem
}

// Store is the batch persistence (satisfied by db.AIBatchStore; faked in tests).
type Store interface {
	CreateJob(ctx context.Context, spec JobSpec) error
	OpenJobs(ctx context.Context) ([]Job, error)
	Items(ctx context.Context, jobID string) ([]JobItem, error)
	MarkItem(ctx context.Context, jobID, customID, status, errMsg string) error
	MarkJob(ctx context.Context, jobID, status, errMsg string) error
	StuckJobs(ctx context.Context, olderThan time.Time) ([]Job, error)
}

// Submit gathers a kind's pending work and submits it as one provider batch. Returns the number of
// items submitted (0 when there was nothing to do). If SubmitBatch fails after Gather has claimed the
// entities, they are reset so they aren't stranded.
func Submit(ctx context.Context, provider ai.BatchProvider, store Store, kind Kind, window string) (int, error) {
	items, err := kind.Gather(ctx)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}

	reqs := make([]ai.BatchRequest, 0, len(items))
	jobItems := make([]JobItem, 0, len(items))
	refIDs := make([]string, 0, len(items))
	for _, it := range items {
		cid := kind.Name() + ":" + it.RefID
		it.Request.CustomID = cid
		reqs = append(reqs, it.Request)
		jobItems = append(jobItems, JobItem{CustomID: cid, RefID: it.RefID})
		refIDs = append(refIDs, it.RefID)
	}

	job, serr := provider.SubmitBatch(ctx, reqs, ai.BatchOptions{CompletionWindow: window, DisplayName: kind.Name()})
	if serr != nil {
		if rerr := kind.Reset(ctx, refIDs); rerr != nil {
			slog.Error("aibatch: reset after submit failure failed", "kind", kind.Name(), "err", rerr)
		}
		return 0, serr
	}
	if cerr := store.CreateJob(ctx, JobSpec{
		Kind: kind.Name(), Provider: job.Provider, Model: job.Model, ProviderBatchID: job.ID, Items: jobItems,
	}); cerr != nil {
		// The batch is live at the provider but we couldn't record it — reset so a later round resubmits.
		if rerr := kind.Reset(ctx, refIDs); rerr != nil {
			slog.Error("aibatch: reset after persist failure failed", "kind", kind.Name(), "err", rerr)
		}
		return 0, cerr
	}
	return len(items), nil
}

// SubmitAll submits every registered kind that has pending work. Returns the total items submitted.
// One kind's failure doesn't stop the others. This is the generic submit entry point an ops-scheduler
// task wraps — adding a batch operation is registering a Kind, no task change.
func SubmitAll(ctx context.Context, provider ai.BatchProvider, store Store, reg *Registry, window string) (int, error) {
	total := 0
	var firstErr error
	for _, kind := range reg.All() {
		n, err := Submit(ctx, provider, store, kind, window)
		if err != nil {
			slog.Error("aibatch: submit kind failed", "kind", kind.Name(), "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		total += n
	}
	return total, firstErr
}

// Collect polls every open job, applies completed results, and resets entities of failed/expired
// batches. It never fails the whole pass on one bad job.
func Collect(ctx context.Context, provider ai.BatchProvider, store Store, reg *Registry) error {
	jobs, err := store.OpenJobs(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, job := range jobs {
		if err := collectJob(ctx, provider, store, reg, job); err != nil {
			slog.Error("aibatch: collect job failed", "job", job.ID, "kind", job.Kind, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func collectJob(ctx context.Context, provider ai.BatchProvider, store Store, reg *Registry, job Job) error {
	batch, err := provider.GetBatch(ctx, job.ProviderBatchID)
	if err != nil {
		return err
	}
	if !batch.Done {
		return nil // still running — check again next round
	}
	kind, ok := reg.Get(job.Kind)
	if !ok {
		return store.MarkJob(ctx, job.ID, "failed", "no registered kind: "+job.Kind)
	}
	if batch.Status != "completed" {
		return resetJob(ctx, store, kind, job, terminalStatus(batch.Status))
	}

	results, rerr := provider.GetBatchResults(ctx, job.ProviderBatchID)
	if errors.Is(rerr, ai.ErrBatchOutputExpired) {
		return resetJob(ctx, store, kind, job, "expired")
	}
	if rerr != nil {
		return rerr
	}

	items, ierr := store.Items(ctx, job.ID)
	if ierr != nil {
		return ierr
	}
	refByCustom := make(map[string]string, len(items))
	for _, it := range items {
		refByCustom[it.CustomID] = it.RefID
	}
	for _, res := range results {
		refID, ok := refByCustom[res.CustomID]
		if !ok {
			continue // unknown custom id — ignore
		}
		var applyErr error
		if res.Error != "" {
			applyErr = errors.New(res.Error)
		}
		status, emsg := "applied", ""
		if aerr := kind.Apply(ctx, refID, res.Output, applyErr); aerr != nil {
			status, emsg = "failed", aerr.Error()
		}
		if merr := store.MarkItem(ctx, job.ID, res.CustomID, status, emsg); merr != nil {
			slog.Warn("aibatch: mark item failed", "job", job.ID, "custom_id", res.CustomID, "err", merr)
		}
	}
	return store.MarkJob(ctx, job.ID, "completed", "")
}

// ResetStuck reclaims jobs that have been 'submitted' longer than the cutoff (a batch that never
// finished or whose completion we missed), returning their entities to pending. Cutoff should be past
// the provider's completion window (e.g. 26h for a 24h batch window).
func ResetStuck(ctx context.Context, store Store, reg *Registry, olderThan time.Time) error {
	jobs, err := store.StuckJobs(ctx, olderThan)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		kind, ok := reg.Get(job.Kind)
		if !ok {
			_ = store.MarkJob(ctx, job.ID, "expired", "stuck, no registered kind")
			continue
		}
		if err := resetJob(ctx, store, kind, job, "expired"); err != nil {
			slog.Error("aibatch: reset stuck job failed", "job", job.ID, "err", err)
		}
	}
	return nil
}

func resetJob(ctx context.Context, store Store, kind Kind, job Job, status string) error {
	items, err := store.Items(ctx, job.ID)
	if err == nil && len(items) > 0 {
		refs := make([]string, 0, len(items))
		for _, it := range items {
			refs = append(refs, it.RefID)
		}
		if rerr := kind.Reset(ctx, refs); rerr != nil {
			slog.Error("aibatch: reset entities failed", "job", job.ID, "kind", kind.Name(), "err", rerr)
		}
	}
	return store.MarkJob(ctx, job.ID, status, "")
}

// terminalStatus maps a provider's non-completed terminal status to our stored set.
func terminalStatus(providerStatus string) string {
	if providerStatus == "expired" {
		return "expired"
	}
	return "failed"
}
