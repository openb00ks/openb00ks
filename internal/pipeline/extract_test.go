package pipeline

import (
	"strings"
	"testing"
)

func i64(v int64) *int64   { return &v }
func str(v string) *string { return &v }

func TestBuildExtractRequest(t *testing.T) {
	sys, user, schema := BuildExtractRequest("ACME\nTotal 12.34")
	if !strings.Contains(sys, "INTEGER CENTS") || !strings.Contains(sys, "never guess") {
		t.Fatalf("system prompt missing key rules")
	}
	if !strings.Contains(user, "ACME") {
		t.Fatalf("user prompt should carry the OCR text")
	}
	if schema != ExtractSchema || !strings.Contains(schema, "additionalProperties") {
		t.Fatalf("schema not the strict ExtractSchema")
	}
}

func TestParseExtract(t *testing.T) {
	raw := `{"vendor_name":"ACME","date":"2026-07-19","currency":"USD","subtotal_cents":1000,"tax_cents":80,"total_cents":1080,"line_items":[],"confidence":0.9}`
	r, err := ParseExtract(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.VendorName == nil || *r.VendorName != "ACME" || r.TotalCents == nil || *r.TotalCents != 1080 {
		t.Fatalf("parsed wrong: %+v", r)
	}

	// Tolerates a markdown code fence.
	if _, err := ParseExtract("```json\n" + raw + "\n```"); err != nil {
		t.Fatalf("fenced parse: %v", err)
	}
	// Rejects garbage.
	if _, err := ParseExtract("not json"); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestValidateExtract(t *testing.T) {
	clean := ExtractResult{
		SubtotalCents: i64(1000), TaxCents: i64(80), TotalCents: i64(1080),
		Date: str("2026-07-19"), Currency: str("USD"), Confidence: 0.9,
		LineItems: []LineItem{{Description: "widget", AmountCents: 600}, {Description: "gadget", AmountCents: 400}},
	}
	if issues := ValidateExtract(clean); len(issues) != 0 {
		t.Fatalf("clean receipt flagged: %v", issues)
	}

	cases := map[string]ExtractResult{
		"total mismatch":         {SubtotalCents: i64(1000), TaxCents: i64(80), TotalCents: i64(2000), Confidence: 0.9},
		"line items != subtotal": {SubtotalCents: i64(1000), Confidence: 0.9, LineItems: []LineItem{{Description: "x", AmountCents: 300}}},
		"bad date":               {Date: str("07/19/2026"), Confidence: 0.9},
		"bad currency":           {Currency: str("Dollars"), Confidence: 0.9},
		"confidence range":       {Confidence: 1.5},
	}
	for name, r := range cases {
		if issues := ValidateExtract(r); len(issues) == 0 {
			t.Fatalf("%s: expected a validation issue, got none", name)
		}
	}

	// Rounding within tolerance is allowed.
	rounded := ExtractResult{SubtotalCents: i64(999), TaxCents: i64(80), TotalCents: i64(1080), Confidence: 0.9}
	if issues := ValidateExtract(rounded); len(issues) != 0 {
		t.Fatalf("1-cent rounding should pass, got %v", issues)
	}
}

func TestGateExtract(t *testing.T) {
	good := ExtractResult{SubtotalCents: i64(1000), TaxCents: i64(80), TotalCents: i64(1080), Confidence: 0.90}
	if out := GateExtract(good); !out.Advance || out.Status != "ok" {
		t.Fatalf("good extraction should advance, got %+v", out)
	}
	// Valid numbers but low confidence → review.
	lowConf := ExtractResult{TotalCents: i64(1080), Confidence: 0.40}
	if out := GateExtract(lowConf); out.Advance {
		t.Fatalf("low-confidence extraction must not advance")
	}
	// High confidence but broken arithmetic → review (a wrong write is expensive).
	broken := ExtractResult{SubtotalCents: i64(1000), TaxCents: i64(80), TotalCents: i64(9999), Confidence: 0.99}
	if out := GateExtract(broken); out.Advance || len(out.Issues) == 0 {
		t.Fatalf("broken arithmetic must park for review even at high confidence, got %+v", out)
	}
}
