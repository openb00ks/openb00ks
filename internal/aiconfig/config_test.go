package aiconfig

import (
	"context"
	"testing"

	"github.com/openb00ks/openb00ks/internal/config"
)

func TestResolverResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cfg           config.Config
		wantAvailable bool
		wantProvider  string
		wantSource    Source
	}{
		{
			name: "no provider configured",
			cfg: config.Config{
				AIProvider: "",
			},
			wantAvailable: false,
			wantProvider:  "rules",
			wantSource:    SourceNone,
		},
		{
			name: "provider explicitly set to none",
			cfg: config.Config{
				AIProvider: "none",
			},
			wantAvailable: false,
			wantProvider:  "rules",
			wantSource:    SourceNone,
		},
		{
			name: "openai provider without api key",
			cfg: config.Config{
				AIProvider:   "openai",
				OpenAIAPIKey: "",
			},
			wantAvailable: false,
			wantProvider:  "rules",
			wantSource:    SourceNone,
		},
		{
			name: "openai provider with api key",
			cfg: config.Config{
				AIProvider:   "openai",
				OpenAIAPIKey: "sk-test-key",
				OpenAIModel:  "gpt-5-nano",
			},
			wantAvailable: true,
			wantProvider:  "openai",
			wantSource:    SourceSystem,
		},
		{
			name: "openai provider with api key no model",
			cfg: config.Config{
				AIProvider:   "openai",
				OpenAIAPIKey: "sk-test-key",
			},
			wantAvailable: true,
			wantProvider:  "openai",
			wantSource:    SourceSystem,
		},
		{
			name: "unknown provider degrades to rules",
			cfg: config.Config{
				AIProvider: "unsupported",
			},
			wantAvailable: false,
			wantProvider:  "rules",
			wantSource:    SourceNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewResolver(tc.cfg)
			result := resolver.Resolve(context.Background(), "test-tenant-id")

			if result.Available != tc.wantAvailable {
				t.Errorf("Available = %v, want %v", result.Available, tc.wantAvailable)
			}
			if result.Provider != tc.wantProvider {
				t.Errorf("Provider = %q, want %q", result.Provider, tc.wantProvider)
			}
			if result.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", result.Source, tc.wantSource)
			}
		})
	}
}

func TestAIConfigIsAIAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config AIConfig
		want   bool
	}{
		{
			name: "available openai",
			config: AIConfig{
				Available: true,
				Provider:  "openai",
			},
			want: true,
		},
		{
			name: "not available rules",
			config: AIConfig{
				Available: false,
				Provider:  "rules",
			},
			want: false,
		},
		{
			name: "available but provider is rules",
			config: AIConfig{
				Available: true,
				Provider:  "rules",
			},
			want: false,
		},
		{
			name: "available but provider is none",
			config: AIConfig{
				Available: true,
				Provider:  "none",
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.config.IsAIAvailable(); got != tc.want {
				t.Errorf("IsAIAvailable() = %v, want %v", got, tc.want)
			}
		})
	}
}
