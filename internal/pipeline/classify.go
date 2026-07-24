package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AccountRef is one candidate general-ledger account offered to the classifier. Decoupled from the DB
// model so the stage stays pure/testable; the caller maps its accounts into these.
type AccountRef struct {
	Code string
	Name string
	Type string // asset|liability|equity|revenue|expense (advisory)
}

// ClassifyResult is the classify-account stage output: exactly one account code from the offered set.
type ClassifyResult struct {
	AccountCode string  `json:"account_code"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
}

const classifySystem = `You assign a single general-ledger account to a purchase receipt.
Choose EXACTLY ONE account_code from the chart of accounts listed below. Never invent a code and never
return free text — the code must appear verbatim in the list. If nothing fits well, pick the closest
account and lower your confidence. confidence is your honest 0.0–1.0 certainty; ambiguous receipts
score low. Return ONLY valid JSON matching the schema.

Chart of accounts:
`

// ClassifySchema is the strict schema for the classify stage.
const ClassifySchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["account_code","confidence","reason"],
  "properties": {
    "account_code": {"type": "string"},
    "confidence": {"type": "number"},
    "reason": {"type": "string"}
  }
}`

// BuildClassifyRequest builds the (system, user, schema). The account list is appended to the system
// prompt (taxonomy injection) so the model can only choose an existing code.
func BuildClassifyRequest(r ExtractResult, accounts []AccountRef) (system, user, schema string) {
	var b strings.Builder
	b.WriteString(classifySystem)
	for _, a := range accounts {
		fmt.Fprintf(&b, "- %s — %s (%s)\n", a.Code, a.Name, a.Type)
	}
	return b.String(), "Receipt:\n" + summarizeForClassify(r), ClassifySchema
}

// summarizeForClassify renders the small, relevant slice of the extraction the classifier needs.
func summarizeForClassify(r ExtractResult) string {
	var b strings.Builder
	if r.VendorName != nil {
		fmt.Fprintf(&b, "vendor: %s\n", *r.VendorName)
	}
	if r.TotalCents != nil {
		fmt.Fprintf(&b, "total_cents: %d\n", *r.TotalCents)
	}
	if len(r.LineItems) > 0 {
		b.WriteString("line_items:\n")
		for _, li := range r.LineItems {
			fmt.Fprintf(&b, "  - %s\n", li.Description)
		}
	}
	return b.String()
}

// ParseClassify decodes the model response.
func ParseClassify(raw string) (ClassifyResult, error) {
	var r ClassifyResult
	if err := json.Unmarshal([]byte(stripCodeFence(raw)), &r); err != nil {
		return ClassifyResult{}, fmt.Errorf("classify: invalid JSON: %w", err)
	}
	return r, nil
}

// ValidateClassify enforces that the chosen code is one that was actually offered — the model cannot
// persist a free-text or hallucinated account.
func ValidateClassify(r ClassifyResult, accounts []AccountRef) []string {
	var issues []string
	if r.Confidence < 0 || r.Confidence > 1 {
		issues = append(issues, fmt.Sprintf("confidence %.2f out of range", r.Confidence))
	}
	found := false
	for _, a := range accounts {
		if a.Code == r.AccountCode {
			found = true
			break
		}
	}
	if !found {
		issues = append(issues, fmt.Sprintf("account_code %q is not in the offered chart of accounts", r.AccountCode))
	}
	return issues
}

// GateClassify applies the confidence gate + validation.
func GateClassify(r ClassifyResult, accounts []AccountRef) Outcome {
	return Gate(r.Confidence, ClassifyConfidenceMin, ValidateClassify(r, accounts))
}
