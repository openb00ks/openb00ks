package ops

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type completion struct {
	name, status, errMsg string
}

type fakeStore struct {
	ensured   map[string]bool
	due       []string
	completed []completion
}

func newFakeStore() *fakeStore { return &fakeStore{ensured: map[string]bool{}} }

func (s *fakeStore) Ensure(_ context.Context, name string, _ time.Duration, _ bool) error {
	s.ensured[name] = true
	return nil
}

func (s *fakeStore) ClaimDue(_ context.Context, _ time.Duration) ([]string, error) {
	d := s.due
	s.due = nil // hand out once
	return d, nil
}

func (s *fakeStore) Complete(_ context.Context, name, status, errMsg string, _ time.Duration) error {
	s.completed = append(s.completed, completion{name, status, errMsg})
	return nil
}

func quietScheduler(store Store) *Scheduler {
	return NewScheduler(store, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func TestScheduler_RunsDueTaskAndRecordsSuccess(t *testing.T) {
	store := newFakeStore()
	store.due = []string{"t"}
	s := quietScheduler(store)

	ran := 0
	s.Register(Task{Name: "t", Run: func(context.Context) error { ran++; return nil }})
	s.runDue(context.Background())

	if ran != 1 {
		t.Fatalf("task ran %d times, want 1", ran)
	}
	if len(store.completed) != 1 || store.completed[0].status != "success" {
		t.Fatalf("completion = %+v, want one success", store.completed)
	}
}

func TestScheduler_RecordsHandlerError(t *testing.T) {
	store := newFakeStore()
	s := quietScheduler(store)
	s.Register(Task{Name: "t", Run: func(context.Context) error { return errors.New("boom") }})

	s.execute(context.Background(), "t")

	if len(store.completed) != 1 || store.completed[0].status != "error" || store.completed[0].errMsg != "boom" {
		t.Fatalf("completion = %+v, want error/boom", store.completed)
	}
}

func TestScheduler_UnregisteredTaskIsCompletedNotSpun(t *testing.T) {
	store := newFakeStore()
	s := quietScheduler(store)
	// No Register — simulate a stale row whose handler was removed.
	s.execute(context.Background(), "ghost")

	if len(store.completed) != 1 || store.completed[0].status != "error" {
		t.Fatalf("stale task should be completed with error, got %+v", store.completed)
	}
}

func TestScheduler_RegisterIgnoresInvalid(t *testing.T) {
	s := quietScheduler(newFakeStore())
	s.Register(Task{Name: "", Run: func(context.Context) error { return nil }}) // no name
	s.Register(Task{Name: "x"})                                                 // no Run
	if len(s.tasks) != 0 {
		t.Fatalf("invalid tasks should be ignored, have %d", len(s.tasks))
	}
}
