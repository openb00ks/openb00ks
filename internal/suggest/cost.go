package suggest

type Pricing struct {
	InputCentsPer1KTokens  int64
	OutputCentsPer1KTokens int64
}

func EstimateCostCents(promptTokens, completionTokens int64, pricing Pricing) int64 {
	if pricing.InputCentsPer1KTokens <= 0 && pricing.OutputCentsPer1KTokens <= 0 {
		return 0
	}
	inputCost := (promptTokens * pricing.InputCentsPer1KTokens) / 1000
	outputCost := (completionTokens * pricing.OutputCentsPer1KTokens) / 1000
	return inputCost + outputCost
}
