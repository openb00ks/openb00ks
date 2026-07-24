package search

import (
	"testing"

	"github.com/openb00ks/openb00ks/internal/config"
)

func TestNewFromConfigNoopDefault(t *testing.T) {
	t.Parallel()

	provider, err := NewFromConfig(config.Config{})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if _, ok := provider.(NoopProvider); !ok {
		t.Fatalf("expected no-op provider, got %T", provider)
	}
}

func TestNewFromConfigRejectsIncompleteTypesense(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfig(config.Config{SearchProvider: "typesense"})
	if err == nil {
		t.Fatalf("expected incomplete typesense config to fail")
	}
}
