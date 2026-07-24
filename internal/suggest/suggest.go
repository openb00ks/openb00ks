package suggest

import (
	"context"
	"errors"
	"sync"
)

type Suggestion struct {
	EntityID   string
	Confidence float64
	Reason     string
	Entries    []SuggestedEntry
}

type Provider interface {
	Suggest(ctx context.Context, input Input) (Suggestion, error)
}

type Input struct {
	ReceiptID   string
	VendorText  string
	Extracted   string
	EntityHints []string
}

type SuggestedEntry struct {
	AccountID   string
	DebitCents  int64
	CreditCents int64
}

// Driver defines a provider factory with named registration.
type Driver interface {
	Name() string
	Open(config Config) (Provider, error)
}

type Config struct {
	Provider string
	APIKey   string
	Model    string
}

var (
	ErrUnknownDriver = errors.New("unknown provider driver")
)

var (
	driverMu sync.RWMutex
	drivers  = make(map[string]Driver)
)

func Register(d Driver) {
	if d == nil || d.Name() == "" {
		return
	}
	driverMu.Lock()
	defer driverMu.Unlock()
	drivers[d.Name()] = d
}

func Open(name string, cfg Config) (Provider, error) {
	driverMu.RLock()
	d := drivers[name]
	driverMu.RUnlock()
	if d == nil {
		return nil, ErrUnknownDriver
	}
	return d.Open(cfg)
}
