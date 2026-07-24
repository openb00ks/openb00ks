package aiconfig

import (
	"context"

	"github.com/openb00ks/openb00ks/internal/config"
)

// Source indicates where the AI configuration came from.
type Source string

const (
	SourceNone   Source = "none"   // No AI configuration available
	SourceSystem Source = "system" // System-level config (env vars)
	SourceTenant Source = "tenant" // Tenant-provided BYOK (future)
)

// AIConfig represents the resolved AI configuration for a request.
type AIConfig struct {
	Available     bool   // Whether AI is available for use
	Provider      string // Provider name: "openai", "anthropic", "rules", "none"
	APIKey        string // The API key to use
	Model         string // The model to use
	Source        Source // Where the config came from
	LimitExceeded bool   // Whether usage limits have been exceeded (future)
}

// Resolver resolves AI configuration for a tenant.
type Resolver struct {
	cfg config.Config
}

// NewResolver creates a new AI config resolver.
func NewResolver(cfg config.Config) *Resolver {
	return &Resolver{cfg: cfg}
}

// Resolve determines the AI configuration to use for a given tenant.
// Phase 3: Only checks system configuration (env vars).
// Future phases will add tenant BYOK lookup and usage limit checking.
func (r *Resolver) Resolve(ctx context.Context, tenantID string) AIConfig {
	// Phase 3: System-only resolution
	return r.resolveSystem()
}

// resolveSystem returns AI config from system environment variables.
func (r *Resolver) resolveSystem() AIConfig {
	provider := r.cfg.AIProvider

	// No AI configured at system level
	if provider == "" || provider == "none" {
		return AIConfig{
			Available: false,
			Provider:  "rules",
			Source:    SourceNone,
		}
	}

	// Provider configured but check for API key
	switch provider {
	case "openai":
		if r.cfg.OpenAIAPIKey == "" {
			// Provider set but no key - degrade to rules
			return AIConfig{
				Available: false,
				Provider:  "rules",
				Source:    SourceNone,
			}
		}
		return AIConfig{
			Available: true,
			Provider:  "openai",
			APIKey:    r.cfg.OpenAIAPIKey,
			Model:     r.cfg.OpenAIModel,
			Source:    SourceSystem,
		}
	default:
		// Unknown provider - degrade to rules
		return AIConfig{
			Available: false,
			Provider:  "rules",
			Source:    SourceNone,
		}
	}
}

// IsAIAvailable returns true if AI suggestions can be attempted.
func (c AIConfig) IsAIAvailable() bool {
	return c.Available && c.Provider != "rules" && c.Provider != "none"
}
