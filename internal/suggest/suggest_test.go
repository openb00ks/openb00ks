package suggest

import (
	"context"
	"errors"
	"testing"
)

type testDriver struct{ name string }

type testProvider struct{}

func (d testDriver) Name() string { return d.name }

func (d testDriver) Open(cfg Config) (Provider, error) { return testProvider{}, nil }

func (testProvider) Suggest(ctx context.Context, input Input) (Suggestion, error) {
	return Suggestion{}, nil
}

func TestRegisterAndOpen(t *testing.T) {
	// Reset drivers map for test isolation.
	drivers = make(map[string]Driver)

	Register(testDriver{name: "test"})
	p, err := Open("test", Config{})
	if err != nil {
		t.Fatalf("expected provider, got error: %v", err)
	}
	if p == nil {
		t.Fatal("expected provider")
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	drivers = make(map[string]Driver)
	_, err := Open("missing", Config{})
	if !errors.Is(err, ErrUnknownDriver) {
		t.Fatalf("expected ErrUnknownDriver, got %v", err)
	}
}

func TestRegisterIgnoresEmptyName(t *testing.T) {
	drivers = make(map[string]Driver)
	Register(testDriver{name: ""})
	if len(drivers) != 0 {
		t.Fatalf("expected no drivers, got %d", len(drivers))
	}
}

func TestNewOpenAIProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewOpenAIProvider("test-key", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}
	if provider.ProviderName() != "openai" {
		t.Fatalf("ProviderName() = %q, want openai", provider.ProviderName())
	}
	if provider.ModelName() != "gpt-4o-mini" {
		t.Fatalf("ModelName() = %q, want gpt-4o-mini", provider.ModelName())
	}
}
