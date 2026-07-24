package db

import "testing"

func TestStatementExpectedEndingBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		start    int64
		inflow   int64
		outflow  int64
		expected int64
	}{
		{name: "bank asset increases with inflow", start: 10_000, inflow: 2_500, outflow: 1_000, expected: 11_500},
		{name: "credit card liability uses negative signed balance", start: -10_000, inflow: 5_000, outflow: 2_500, expected: -7_500},
		{name: "zero activity preserves balance", start: 123, expected: 123},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StatementExpectedEndingBalance(tt.start, tt.inflow, tt.outflow)
			if got != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestStatementDifferenceCents(t *testing.T) {
	t.Parallel()

	if got := StatementDifferenceCents(12_000, 11_500); got != 500 {
		t.Fatalf("expected positive difference, got %d", got)
	}
	if got := StatementDifferenceCents(11_500, 12_000); got != -500 {
		t.Fatalf("expected negative difference, got %d", got)
	}
}
