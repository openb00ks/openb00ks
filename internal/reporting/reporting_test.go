package reporting

import "testing"

func TestNormalizeType(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"asset":       "asset",
		"assets":      "asset",
		"liability":   "liability",
		"liabilities": "liability",
		"equity":      "equity",
		"income":      "income",
		"revenue":     "income",
		"expense":     "expense",
		"expenses":    "expense",
		"Expenses":    "expense", // case-insensitive
		" income ":    "income",  // trimmed
		"other":       "other",
		"":            "other",
	}
	for input, want := range cases {
		if got := NormalizeType(input); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalBalance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		accountType   string
		debit, credit int64
		want          int64
	}{
		{"asset", 500, 200, 300},     // debit-normal
		{"expense", 500, 200, 300},   // debit-normal
		{"income", 200, 500, 300},    // credit-normal
		{"liability", 200, 500, 300}, // credit-normal
		{"equity", 200, 500, 300},    // credit-normal
		{"other", 200, 500, 300},     // unknown -> credit-normal
		{"asset", 100, 100, 0},
	}
	for _, tc := range cases {
		if got := NormalBalance(tc.accountType, tc.debit, tc.credit); got != tc.want {
			t.Errorf("NormalBalance(%q, %d, %d) = %d, want %d", tc.accountType, tc.debit, tc.credit, got, tc.want)
		}
	}
}

func TestSplitDebitCredit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		net            int64
		wantDr, wantCr int64
	}{
		{300, 300, 0},
		{-300, 0, 300},
		{0, 0, 0},
	}
	for _, tc := range cases {
		dr, cr := SplitDebitCredit(tc.net)
		if dr != tc.wantDr || cr != tc.wantCr {
			t.Errorf("SplitDebitCredit(%d) = (%d, %d), want (%d, %d)", tc.net, dr, cr, tc.wantDr, tc.wantCr)
		}
	}
}
