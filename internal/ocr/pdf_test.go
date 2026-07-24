package ocr

import (
	"strings"
	"testing"
)

func TestSufficientText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"whitespace only", "   \n\t ", false},
		{"too short even with digits", "Total 12", false},                // 8 runes < 40
		{"long but no digits", strings.Repeat("abcde ", 20), false},      // 120 runes, 0 digits
		{"long, one digit only", strings.Repeat("ab ", 20) + "7", false}, // digits < 3
		{"real receipt", "ACME STORE\n2026-07-21\nCoffee 4.50\nTotal 4.86", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := SufficientText(c.in); got != c.want {
				t.Errorf("SufficientText(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestExtractPDFText_BadInput(t *testing.T) {
	t.Parallel()
	if _, err := ExtractPDFText(nil); err == nil {
		t.Error("expected an error for empty input")
	}
	// Non-PDF bytes must yield an error (parse error or a recovered panic) — never a crash.
	if _, err := ExtractPDFText([]byte("this is definitely not a pdf")); err == nil {
		t.Error("expected an error for non-pdf bytes")
	}
}
