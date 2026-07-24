package db

import (
	"context"
	"testing"
	"time"
)

func TestManagerReadyWithoutDB(t *testing.T) {
	manager := NewManager("")
	if err := manager.Ready(context.Background()); err == nil {
		t.Fatal("expected error when database is unavailable")
	}
}

func TestManagerStartConnects(t *testing.T) {
	manager := NewManager("dsn")
	manager.backoff = 10 * time.Millisecond
	manager.maxBackoff = 10 * time.Millisecond
	called := make(chan struct{}, 1)
	manager.open = func(string) (*DB, error) {
		return &DB{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx, func(*DB) {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("expected manager to call onConnect")
	}
}
