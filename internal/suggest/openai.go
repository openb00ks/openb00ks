package suggest

import (
	ai "github.com/spectrum-labs-tech/ai"

	_ "github.com/spectrum-labs-tech/ai/drivers/openai"
)

// NewOpenAIProvider constructs the shared OpenAI provider used by Open B00KS.
// The provider is registered through the external ai package and exposed here
// so the rest of the app does not need to know the driver registry details.
func NewOpenAIProvider(apiKey, model string) (ai.Provider, error) {
	return ai.New(&ai.Config{
		Provider: "openai",
		APIKey:   apiKey,
		Model:    model,
	})
}
