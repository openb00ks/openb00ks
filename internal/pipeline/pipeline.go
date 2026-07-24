// Package pipeline implements the decomposed receipt → journal-entry stages (see
// docs/receipt-pipeline.md). Each AI stage is expressed as three PURE functions —
// Build<Stage>Request (system+user prompt + strict JSON schema), Parse<Stage> (JSON → typed
// result), and Validate<Stage> (deterministic checks) — plus a confidence Gate. Keeping the logic
// pure makes every stage unit-testable without an AI provider or a database, and lets the same
// request builders feed both production and the eval harness (the frozen-prompt rule).
package pipeline

import "strings"

// Per-stage hard confidence gates (below the bar → human review, never auto-advance).
const (
	ExtractConfidenceMin  = 0.75
	ClassifyConfidenceMin = 0.80
	VendorConfidenceMin   = 0.85
)

// Outcome is a stage's gate decision.
type Outcome struct {
	Advance bool     // true → move to the next stage; false → park for review
	Status  string   // "ok" | "needs_review"
	Issues  []string // deterministic validation problems (empty when ok)
}

// Gate advances only when the model was confident AND every deterministic check passed. Any validation
// issue parks the record regardless of confidence — a wrong write is expensive.
func Gate(confidence, min float64, issues []string) Outcome {
	if len(issues) == 0 && confidence >= min {
		return Outcome{Advance: true, Status: "ok"}
	}
	return Outcome{Advance: false, Status: "needs_review", Issues: issues}
}

// stripCodeFence removes a leading/trailing ```/```json markdown fence if the model wrapped its JSON
// (strict structured output should not, but be defensive).
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 { // drop the "json" language tag line
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
