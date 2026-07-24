package db

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("database unavailable")

type Manager struct {
	dsn        string
	open       func(string) (*DB, error)
	backoff    time.Duration
	maxBackoff time.Duration
	mu         sync.RWMutex
	db         *DB
}

func NewManager(dsn string) *Manager {
	return &Manager{
		dsn:        dsn,
		open:       Open,
		backoff:    2 * time.Second,
		maxBackoff: 30 * time.Second,
	}
}

func (m *Manager) DB() *DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db
}

func (m *Manager) DSN() string {
	return m.dsn
}

func (m *Manager) SetDB(dbConn *DB) {
	m.mu.Lock()
	m.db = dbConn
	m.mu.Unlock()
}

func (m *Manager) Ready(ctx context.Context) error {
	dbConn := m.DB()
	if dbConn == nil {
		return ErrUnavailable
	}
	return dbConn.Ready(ctx)
}

func (m *Manager) Start(ctx context.Context, onConnect func(*DB)) {
	go func() {
		backoff := m.backoff
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			dbConn := m.DB()
			if dbConn != nil {
				if err := dbConn.Ready(ctx); err == nil {
					time.Sleep(backoff)
					continue
				}
			}

			if m.dsn == "" {
				time.Sleep(backoff)
				continue
			}

			conn, err := m.open(m.dsn)
			if err == nil {
				m.SetDB(conn)
				if onConnect != nil {
					onConnect(conn)
				}
				backoff = m.backoff
				time.Sleep(backoff)
				continue
			}

			backoff *= 2
			if backoff > m.maxBackoff {
				backoff = m.maxBackoff
			}
			time.Sleep(backoff)
		}
	}()
}
