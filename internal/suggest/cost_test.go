package suggest

import "testing"

func TestEstimateCostCents(t *testing.T) {
	pricing := Pricing{InputCentsPer1KTokens: 30, OutputCentsPer1KTokens: 60}
	cost := EstimateCostCents(1000, 500, pricing)
	if cost != 60 {
		t.Fatalf("expected 60, got %d", cost)
	}
}

func TestEstimateCostCentsZeroPricing(t *testing.T) {
	pricing := Pricing{}
	cost := EstimateCostCents(1000, 1000, pricing)
	if cost != 0 {
		t.Fatalf("expected 0, got %d", cost)
	}
}

func TestEstimateCostCentsPartialPricing(t *testing.T) {
	pricing := Pricing{InputCentsPer1KTokens: 10}
	cost := EstimateCostCents(2500, 1000, pricing)
	if cost != 25 {
		t.Fatalf("expected 25, got %d", cost)
	}
}
