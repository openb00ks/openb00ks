package httpapi

import (
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestFallbackSuggestionCandidatesIncludesRoleHints(t *testing.T) {
	t.Parallel()

	got := fallbackSuggestionCandidates(suggestRequest{
		Text:      "  ",
		Extracted: "",
		Context:   "office internet",
	}, "Acme Internet", "internet", "utilities")

	if len(got) != 4 {
		t.Fatalf("expected 4 candidates, got %d (%v)", len(got), got)
	}
	if got[0] != "office internet" || got[1] != "Acme Internet" || got[2] != "internet" || got[3] != "utilities" {
		t.Fatalf("unexpected candidates: %v", got)
	}
}

func TestAccountRoleHintTokensDedupesAndSorts(t *testing.T) {
	t.Parallel()

	got := accountRoleHintTokens([]models.Account{
		{RoleTags: []string{"internet", "utilities"}},
		{RoleTags: []string{"cell_phone", "internet"}},
	})

	if len(got) != 3 {
		t.Fatalf("expected 3 hints, got %v", got)
	}
	if got[0] != "cell_phone" || got[1] != "internet" || got[2] != "utilities" {
		t.Fatalf("unexpected hints: %v", got)
	}
}
