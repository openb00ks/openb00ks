package pipeline

import (
	"encoding/json"
	"fmt"
	"time"
)

// LineItem is one row on a receipt. Amounts are integer cents (avoids float money).
type LineItem struct {
	Description string   `json:"description"`
	Quantity    *float64 `json:"quantity"`
	AmountCents int64    `json:"amount_cents"`
}

// ExtractResult is the structured output of the extract stage. Pointer fields are nullable: the model
// MUST return null (not a guess) for anything not legible on the receipt.
type ExtractResult struct {
	VendorName    *string    `json:"vendor_name"`
	Date          *string    `json:"date"`     // ISO 8601 (YYYY-MM-DD)
	Currency      *string    `json:"currency"` // ISO 4217
	SubtotalCents *int64     `json:"subtotal_cents"`
	TaxCents      *int64     `json:"tax_cents"`
	TotalCents    *int64     `json:"total_cents"`
	LineItems     []LineItem `json:"line_items"`
	Confidence    float64    `json:"confidence"`
}

const extractSystem = `You extract structured fields from the raw text of a receipt or invoice.
Rules:
- Extract ONLY what is present in the text. If a field is not legible, return null — never guess.
- All monetary amounts are INTEGER CENTS (e.g. $12.34 → 1234). Never a decimal or a string.
- date is ISO 8601 (YYYY-MM-DD); currency is the ISO 4217 code (e.g. USD).
- line_items: include one object per itemized line if the receipt is itemized; otherwise return [].
- Do NOT compute totals or taxes that are not printed. Do NOT invent a vendor.
- confidence is your honest 0.0–1.0 certainty in the WHOLE extraction; a blurry or ambiguous receipt
  scores low.
Return ONLY valid JSON matching the schema.`

// ExtractSchema is the strict JSON schema for the extract stage (additionalProperties:false, all
// required — the model can only emit the shape the stage consumes).
const ExtractSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["vendor_name","date","currency","subtotal_cents","tax_cents","total_cents","line_items","confidence"],
  "properties": {
    "vendor_name": {"type": ["string","null"]},
    "date": {"type": ["string","null"]},
    "currency": {"type": ["string","null"]},
    "subtotal_cents": {"type": ["integer","null"]},
    "tax_cents": {"type": ["integer","null"]},
    "total_cents": {"type": ["integer","null"]},
    "line_items": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["description","quantity","amount_cents"],
        "properties": {
          "description": {"type": "string"},
          "quantity": {"type": ["number","null"]},
          "amount_cents": {"type": "integer"}
        }
      }
    },
    "confidence": {"type": "number"}
  }
}`

// BuildExtractRequest returns the (system, user, schema) for the extract stage. ocrText is the stage-1
// transcription. Shared by production and the eval harness.
func BuildExtractRequest(ocrText string) (system, user, schema string) {
	return extractSystem, "Receipt text:\n" + ocrText, ExtractSchema
}

// ParseExtract decodes the model's JSON response.
func ParseExtract(raw string) (ExtractResult, error) {
	var r ExtractResult
	if err := json.Unmarshal([]byte(stripCodeFence(raw)), &r); err != nil {
		return ExtractResult{}, fmt.Errorf("extract: invalid JSON: %w", err)
	}
	return r, nil
}

// centsTolerance is the rounding slack (in cents) allowed on arithmetic checks — receipts occasionally
// round line items independently of the printed total.
const centsTolerance = 2

// ValidateExtract runs the deterministic checks that a language model can't be trusted to self-enforce.
// Returns human-readable issues; empty == clean.
func ValidateExtract(r ExtractResult) []string {
	var issues []string

	if r.Confidence < 0 || r.Confidence > 1 {
		issues = append(issues, fmt.Sprintf("confidence %.2f out of range", r.Confidence))
	}
	// total == subtotal + tax (when all three are present).
	if r.TotalCents != nil && r.SubtotalCents != nil && r.TaxCents != nil {
		want := *r.SubtotalCents + *r.TaxCents
		if abs64(*r.TotalCents-want) > centsTolerance {
			issues = append(issues, fmt.Sprintf("total %d != subtotal %d + tax %d", *r.TotalCents, *r.SubtotalCents, *r.TaxCents))
		}
	}
	// Σ line items ≈ subtotal (or total when there's no subtotal).
	if len(r.LineItems) > 0 {
		var sum int64
		for _, li := range r.LineItems {
			sum += li.AmountCents
		}
		switch {
		case r.SubtotalCents != nil && abs64(sum-*r.SubtotalCents) > centsTolerance:
			issues = append(issues, fmt.Sprintf("line items sum %d != subtotal %d", sum, *r.SubtotalCents))
		case r.SubtotalCents == nil && r.TotalCents != nil && abs64(sum-*r.TotalCents) > centsTolerance:
			issues = append(issues, fmt.Sprintf("line items sum %d != total %d", sum, *r.TotalCents))
		}
	}
	if r.Date != nil && *r.Date != "" {
		if _, err := time.Parse("2006-01-02", *r.Date); err != nil {
			issues = append(issues, fmt.Sprintf("date %q is not YYYY-MM-DD", *r.Date))
		}
	}
	if r.Currency != nil && *r.Currency != "" && len(*r.Currency) != 3 {
		issues = append(issues, fmt.Sprintf("currency %q is not a 3-letter ISO 4217 code", *r.Currency))
	}
	return issues
}

// GateExtract applies the confidence gate + validation for the extract stage.
func GateExtract(r ExtractResult) Outcome {
	return Gate(r.Confidence, ExtractConfidenceMin, ValidateExtract(r))
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
