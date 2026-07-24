package aibatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spectrum-labs-tech/ai"
)

// --- fakes ---

type fakeProvider struct {
	submitted  []ai.BatchRequest
	submitErr  error
	done       bool
	status     string
	results    []ai.BatchResult
	resultsErr error
}

func (f *fakeProvider) SubmitBatch(_ context.Context, reqs []ai.BatchRequest, _ ai.BatchOptions) (*ai.BatchJob, error) {
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	f.submitted = reqs
	return &ai.BatchJob{ID: "pbatch-1", Provider: "fake", Model: "m"}, nil
}
func (f *fakeProvider) GetBatch(_ context.Context, id string) (*ai.BatchJob, error) {
	return &ai.BatchJob{ID: id, Status: f.status, Done: f.done}, nil
}
func (f *fakeProvider) GetBatchResults(_ context.Context, _ string) ([]ai.BatchResult, error) {
	return f.results, f.resultsErr
}
func (f *fakeProvider) CancelBatch(_ context.Context, id string) (*ai.BatchJob, error) {
	return &ai.BatchJob{ID: id, Status: "cancelled", Done: true}, nil
}

// ai.BatchProvider embeds ai.Provider — satisfy the rest.
func (f *fakeProvider) Complete(context.Context, string, string, string, ai.Options) (string, error) {
	return "", nil
}
func (f *fakeProvider) ProviderName() string { return "fake" }
func (f *fakeProvider) ModelName() string    { return "m" }
func (f *fakeProvider) Close() error         { return nil }

type fakeStore struct {
	created     []JobSpec
	open        []Job
	itemsByJob  map[string][]JobItem
	markedJobs  map[string]string // jobID → status
	markedItems map[string]string // jobID+custom → status
	stuck       []Job
}

func newStore() *fakeStore {
	return &fakeStore{itemsByJob: map[string][]JobItem{}, markedJobs: map[string]string{}, markedItems: map[string]string{}}
}
func (s *fakeStore) CreateJob(_ context.Context, spec JobSpec) error {
	s.created = append(s.created, spec)
	id := "job-1"
	s.open = append(s.open, Job{ID: id, Kind: spec.Kind, ProviderBatchID: spec.ProviderBatchID, Status: "submitted"})
	s.itemsByJob[id] = spec.Items
	return nil
}
func (s *fakeStore) OpenJobs(context.Context) ([]Job, error) { return s.open, nil }
func (s *fakeStore) Items(_ context.Context, jobID string) ([]JobItem, error) {
	return s.itemsByJob[jobID], nil
}
func (s *fakeStore) MarkItem(_ context.Context, jobID, customID, status, _ string) error {
	s.markedItems[jobID+"|"+customID] = status
	return nil
}
func (s *fakeStore) MarkJob(_ context.Context, jobID, status, _ string) error {
	s.markedJobs[jobID] = status
	return nil
}
func (s *fakeStore) StuckJobs(context.Context, time.Time) ([]Job, error) { return s.stuck, nil }

type fakeKind struct {
	name      string
	gather    []Item
	gatherErr error
	applied   []string
	reset     []string
}

func (k *fakeKind) Name() string                           { return k.name }
func (k *fakeKind) Gather(context.Context) ([]Item, error) { return k.gather, k.gatherErr }
func (k *fakeKind) Apply(_ context.Context, refID, _ string, _ error) error {
	k.applied = append(k.applied, refID)
	return nil
}
func (k *fakeKind) Reset(_ context.Context, refIDs []string) error {
	k.reset = append(k.reset, refIDs...)
	return nil
}

func req(ref string) Item {
	return Item{RefID: ref, Request: ai.BatchRequest{UserPrompt: "p-" + ref, JSONSchema: "{}"}}
}

// --- tests ---

func TestSubmit_GathersAndCreatesJobWithCustomIDs(t *testing.T) {
	store := newStore()
	prov := &fakeProvider{}
	kind := &fakeKind{name: "receipt-extract", gather: []Item{req("r1"), req("r2")}}

	n, err := Submit(context.Background(), prov, store, kind, "24h")
	if err != nil || n != 2 {
		t.Fatalf("Submit n=%d err=%v", n, err)
	}
	if len(store.created) != 1 || len(store.created[0].Items) != 2 {
		t.Fatalf("job not recorded with 2 items: %+v", store.created)
	}
	if store.created[0].Items[0].CustomID != "receipt-extract:r1" {
		t.Fatalf("custom id = %q", store.created[0].Items[0].CustomID)
	}
	if len(prov.submitted) != 2 || prov.submitted[0].CustomID != "receipt-extract:r1" {
		t.Fatalf("provider requests not tagged with custom ids: %+v", prov.submitted)
	}
}

func TestSubmit_NothingToDo(t *testing.T) {
	store := newStore()
	prov := &fakeProvider{}
	n, err := Submit(context.Background(), prov, store, &fakeKind{name: "k"}, "24h")
	if err != nil || n != 0 {
		t.Fatalf("empty gather: n=%d err=%v", n, err)
	}
	if len(store.created) != 0 || prov.submitted != nil {
		t.Fatal("nothing should have been submitted")
	}
}

func TestSubmit_ProviderError_ResetsClaimedEntities(t *testing.T) {
	store := newStore()
	prov := &fakeProvider{submitErr: errors.New("rate limited")}
	kind := &fakeKind{name: "k", gather: []Item{req("r1"), req("r2")}}

	if _, err := Submit(context.Background(), prov, store, kind, "24h"); err == nil {
		t.Fatal("expected submit error")
	}
	if len(store.created) != 0 {
		t.Fatal("no job should be recorded on submit failure")
	}
	if len(kind.reset) != 2 {
		t.Fatalf("claimed entities must be reset on submit failure, got %v", kind.reset)
	}
}

func TestCollect_Completed_AppliesResultsAndClosesJob(t *testing.T) {
	store := newStore()
	store.open = []Job{{ID: "job-1", Kind: "k", ProviderBatchID: "pbatch-1", Status: "submitted"}}
	store.itemsByJob["job-1"] = []JobItem{{CustomID: "k:r1", RefID: "r1"}, {CustomID: "k:r2", RefID: "r2"}}
	prov := &fakeProvider{done: true, status: "completed", results: []ai.BatchResult{
		{CustomID: "k:r1", Output: "{}"}, {CustomID: "k:r2", Output: "{}"},
	}}
	kind := &fakeKind{name: "k"}
	reg := NewRegistry()
	reg.Register(kind)

	if err := Collect(context.Background(), prov, store, reg); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(kind.applied) != 2 {
		t.Fatalf("both results should be applied, got %v", kind.applied)
	}
	if store.markedJobs["job-1"] != "completed" {
		t.Fatalf("job not marked completed: %v", store.markedJobs)
	}
	if store.markedItems["job-1|k:r1"] != "applied" {
		t.Fatalf("item not marked applied: %v", store.markedItems)
	}
}

func TestCollect_FailedBatch_ResetsEntities(t *testing.T) {
	store := newStore()
	store.open = []Job{{ID: "job-1", Kind: "k", ProviderBatchID: "pbatch-1", Status: "submitted"}}
	store.itemsByJob["job-1"] = []JobItem{{CustomID: "k:r1", RefID: "r1"}}
	prov := &fakeProvider{done: true, status: "failed"}
	kind := &fakeKind{name: "k"}
	reg := NewRegistry()
	reg.Register(kind)

	if err := Collect(context.Background(), prov, store, reg); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(kind.reset) != 1 || kind.reset[0] != "r1" {
		t.Fatalf("failed batch must reset its entities, got %v", kind.reset)
	}
	if store.markedJobs["job-1"] != "failed" {
		t.Fatalf("job should be marked failed, got %v", store.markedJobs)
	}
	if len(kind.applied) != 0 {
		t.Fatal("no results should be applied for a failed batch")
	}
}

func TestCollect_StillRunning_NoOp(t *testing.T) {
	store := newStore()
	store.open = []Job{{ID: "job-1", Kind: "k", ProviderBatchID: "pbatch-1"}}
	prov := &fakeProvider{done: false, status: "in_progress"}
	reg := NewRegistry()
	reg.Register(&fakeKind{name: "k"})
	if err := Collect(context.Background(), prov, store, reg); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(store.markedJobs) != 0 {
		t.Fatal("a still-running batch must not be touched")
	}
}

func TestResetStuck(t *testing.T) {
	store := newStore()
	store.stuck = []Job{{ID: "job-1", Kind: "k", ProviderBatchID: "pbatch-1"}}
	store.itemsByJob["job-1"] = []JobItem{{CustomID: "k:r1", RefID: "r1"}}
	kind := &fakeKind{name: "k"}
	reg := NewRegistry()
	reg.Register(kind)

	if err := ResetStuck(context.Background(), store, reg, time.Now()); err != nil {
		t.Fatalf("ResetStuck: %v", err)
	}
	if len(kind.reset) != 1 || store.markedJobs["job-1"] != "expired" {
		t.Fatalf("stuck job should reset entities + expire: reset=%v jobs=%v", kind.reset, store.markedJobs)
	}
}

func TestSubmitAll_DrivesEveryKind(t *testing.T) {
	store := newStore()
	prov := &fakeProvider{}
	reg := NewRegistry()
	reg.Register(&fakeKind{name: "k1", gather: []Item{req("a")}})
	reg.Register(&fakeKind{name: "k2", gather: []Item{req("b"), req("c")}})

	n, err := SubmitAll(context.Background(), prov, store, reg, "24h")
	if err != nil || n != 3 {
		t.Fatalf("SubmitAll n=%d err=%v (want 3)", n, err)
	}
	if len(store.created) != 2 {
		t.Fatalf("expected a job per kind with work, got %d", len(store.created))
	}
}
