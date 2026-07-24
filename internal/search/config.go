package search

import (
	"fmt"
	"strings"

	"github.com/openb00ks/openb00ks/internal/config"
)

func NewFromConfig(cfg config.Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.SearchProvider)) {
	case "", "none":
		return NoopProvider{}, nil
	case "typesense":
		return NewTypesenseProvider(TypesenseConfig{
			URL:              cfg.TypesenseURL,
			APIKey:           cfg.TypesenseAPIKey,
			CollectionPrefix: cfg.TypesenseCollectionPrefix,
		})
	default:
		return nil, fmt.Errorf("unsupported search provider %q", cfg.SearchProvider)
	}
}

func OptionalFromConfig(cfg config.Config) Provider {
	provider, err := NewFromConfig(cfg)
	if err != nil {
		return NoopProvider{}
	}
	return provider
}
