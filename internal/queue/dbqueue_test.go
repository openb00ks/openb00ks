package queue

import (
	"context"
	"errors"
	"testing"
)

func TestDBQueueReadyNil(t *testing.T) {
	var q *DBQueue
	if err := q.Ready(context.Background()); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("expected ErrQueueUnavailable, got %v", err)
	}
}
