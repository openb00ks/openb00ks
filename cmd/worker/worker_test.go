package main

import (
	"os"
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens([]byte("")); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := estimateTokens([]byte("1234")); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestParseInt64(t *testing.T) {
	payload := map[string]interface{}{
		"n":   12.0,
		"s":   "42",
		"bad": "x",
	}
	if got := parseInt64(payload, "n"); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
	if got := parseInt64(payload, "s"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := parseInt64(payload, "bad"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestBuildEntriesFromSuggestion(t *testing.T) {
	receipt := models.Receipt{TotalCents: 500}

	payload := `{"account_id":"expense","total_cents":500}`
	suggestion := models.ReceiptSuggestion{ParsedJSON: []byte(payload)}
	entries := buildEntriesFromSuggestion(receipt, suggestion, "cash")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].AccountID != "expense" || entries[1].AccountID != "cash" {
		t.Fatalf("unexpected accounts: %+v", entries)
	}

	payload = `{"entries":[{"account_id":"a1","debit_cents":100},{"account_id":"a2","credit_cents":100}]}`
	suggestion = models.ReceiptSuggestion{ParsedJSON: []byte(payload)}
	entries = buildEntriesFromSuggestion(receipt, suggestion, "cash")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestEnvHelpers(t *testing.T) {
	if err := os.Setenv("TEST_ENV", "value"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if got := envOr("TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("expected value, got %s", got)
	}
	if err := os.Unsetenv("TEST_ENV"); err != nil {
		t.Fatalf("unset env: %v", err)
	}
	if got := envOr("TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %s", got)
	}

	if err := os.Setenv("TEST_ENV_INT", "7"); err != nil {
		t.Fatalf("set env int: %v", err)
	}
	if got := envOrInt("TEST_ENV_INT", 3); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
	if err := os.Setenv("TEST_ENV_INT", "bad"); err != nil {
		t.Fatalf("set env int bad: %v", err)
	}
	if got := envOrInt("TEST_ENV_INT", 3); got != 3 {
		t.Fatalf("expected fallback 3, got %d", got)
	}
	if err := os.Unsetenv("TEST_ENV_INT"); err != nil {
		t.Fatalf("unset env int: %v", err)
	}
	if got := envOrInt("TEST_ENV_INT", 3); got != 3 {
		t.Fatalf("expected fallback 3, got %d", got)
	}
}
