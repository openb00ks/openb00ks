package pipeline

import (
	"strings"
	"testing"
)

var testAccounts = []AccountRef{
	{Code: "6000", Name: "Office Supplies", Type: "expense"},
	{Code: "6100", Name: "Meals & Entertainment", Type: "expense"},
	{Code: "6200", Name: "Software Subscriptions", Type: "expense"},
}

func TestBuildClassifyRequest_InjectsAccounts(t *testing.T) {
	r := ExtractResult{VendorName: str("Staples"), TotalCents: i64(4200),
		LineItems: []LineItem{{Description: "printer paper", AmountCents: 4200}}}
	sys, user, schema := BuildClassifyRequest(r, testAccounts)
	for _, code := range []string{"6000", "6100", "6200"} {
		if !strings.Contains(sys, code) {
			t.Fatalf("system prompt missing account %s", code)
		}
	}
	if !strings.Contains(sys, "never invent a code") && !strings.Contains(sys, "Never invent a code") {
		t.Fatalf("system prompt missing the no-invention rule")
	}
	if !strings.Contains(user, "Staples") || !strings.Contains(user, "printer paper") {
		t.Fatalf("user prompt missing receipt summary: %q", user)
	}
	if schema != ClassifySchema {
		t.Fatalf("wrong schema")
	}
}

func TestParseClassify(t *testing.T) {
	r, err := ParseClassify(`{"account_code":"6000","confidence":0.88,"reason":"office paper"}`)
	if err != nil || r.AccountCode != "6000" || r.Confidence != 0.88 {
		t.Fatalf("parse wrong: %+v err=%v", r, err)
	}
}

func TestValidateAndGateClassify(t *testing.T) {
	// Off-list code must be rejected (no free-text/hallucinated accounts).
	off := ClassifyResult{AccountCode: "9999", Confidence: 0.95}
	if issues := ValidateClassify(off, testAccounts); len(issues) == 0 {
		t.Fatal("off-list account_code should be flagged")
	}
	if GateClassify(off, testAccounts).Advance {
		t.Fatal("off-list code must not advance even at high confidence")
	}

	// Valid code, high confidence → advance.
	ok := ClassifyResult{AccountCode: "6200", Confidence: 0.90}
	if out := GateClassify(ok, testAccounts); !out.Advance || out.Status != "ok" {
		t.Fatalf("valid confident classification should advance, got %+v", out)
	}

	// Valid code, low confidence → review.
	low := ClassifyResult{AccountCode: "6200", Confidence: 0.5}
	if GateClassify(low, testAccounts).Advance {
		t.Fatal("low-confidence classification must not advance")
	}
}
