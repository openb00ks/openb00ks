package pipeline

import (
	"context"
	"errors"
	"testing"
)

// fakeAI routes by schema so tests don't depend on call order; vendorCalls counts AI vendor calls.
type fakeAI struct {
	extract, vendor, classify string
	err                       error
	vendorCalls               int
	classifyCalls             int
}

func (f *fakeAI) Complete(_ context.Context, _, _, schema string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	switch schema {
	case ExtractSchema:
		return f.extract, nil
	case VendorSchema:
		f.vendorCalls++
		return f.vendor, nil
	case ClassifySchema:
		f.classifyCalls++
		return f.classify, nil
	default:
		return "", errors.New("unexpected schema")
	}
}

func baseInput() RunInput {
	return RunInput{
		OCRText:       "AWS invoice\nTotal 10.80",
		Accounts:      []AccountRef{{Code: "6200", Name: "Software", Type: "expense"}},
		Vendors:       []VendorRef{{ID: "v2", Name: "Amazon Web Services"}},
		CreditAccount: "2000",
	}
}

func TestRun_HappyPath_UsesAIVendorMatch(t *testing.T) {
	ai := &fakeAI{
		extract:  `{"vendor_name":"AWS","date":"2026-07-19","currency":"USD","subtotal_cents":1000,"tax_cents":80,"total_cents":1080,"line_items":[],"confidence":0.95}`,
		vendor:   `{"vendor_id":"v2","is_new_vendor":false,"confidence":0.9,"reason":"aws"}`,
		classify: `{"account_code":"6200","confidence":0.9,"reason":"cloud"}`,
	}
	res, err := Run(context.Background(), ai, baseInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ready" {
		t.Fatalf("status = %q issues=%v failed=%q", res.Status, res.Issues, res.FailedStage)
	}
	if ai.vendorCalls != 1 {
		t.Fatalf("expected 1 AI vendor call (no exact match), got %d", ai.vendorCalls)
	}
	if res.VendorID == nil || *res.VendorID != "v2" {
		t.Fatalf("vendor not resolved: %v", res.VendorID)
	}
	if res.Entry == nil {
		t.Fatal("expected a built entry")
	}
	if err := AssertBalanced(*res.Entry); err != nil {
		t.Fatalf("built entry not balanced: %v", err)
	}
}

func TestRun_ExactVendorMatch_SkipsAI(t *testing.T) {
	ai := &fakeAI{
		extract:  `{"vendor_name":"Amazon Web Services","total_cents":1080,"line_items":[],"confidence":0.95}`,
		classify: `{"account_code":"6200","confidence":0.9,"reason":"cloud"}`,
	}
	res, err := Run(context.Background(), ai, baseInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ai.vendorCalls != 0 {
		t.Fatalf("exact match should skip the AI vendor call, got %d", ai.vendorCalls)
	}
	if res.Status != "ready" || res.VendorID == nil || *res.VendorID != "v2" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRun_MatchedVendorWithDefaultAccount_SkipsClassify(t *testing.T) {
	in := RunInput{
		OCRText:       "Amazon Web Services\nTotal 10.80",
		Accounts:      []AccountRef{{Code: "6200", Name: "Software"}},
		Vendors:       []VendorRef{{ID: "v2", Name: "Amazon Web Services", DefaultAccount: "6200"}},
		CreditAccount: "2000",
	}
	// classify intentionally omitted from the fake — it must not be called.
	ai := &fakeAI{extract: `{"vendor_name":"Amazon Web Services","total_cents":1080,"line_items":[],"confidence":0.95}`}

	res, err := Run(context.Background(), ai, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ai.vendorCalls != 0 {
		t.Fatalf("exact vendor match should skip the vendor AI, got %d", ai.vendorCalls)
	}
	if ai.classifyCalls != 0 {
		t.Fatalf("a matched vendor with a default account must skip classify, got %d calls", ai.classifyCalls)
	}
	if res.Classify == nil || res.Classify.AccountCode != "6200" {
		t.Fatalf("account should come from the vendor default: %+v", res.Classify)
	}
	if res.Status != "ready" || res.Entry == nil {
		t.Fatalf("should still build a balanced draft: %+v", res)
	}
}

func TestRun_UsesCandidateCallback(t *testing.T) {
	called := ""
	in := RunInput{
		OCRText:       "Amazon\nTotal 10.80",
		Accounts:      []AccountRef{{Code: "6200", Name: "Software"}},
		CreditAccount: "2000",
		Candidates: func(name string) []VendorRef {
			called = name
			return []VendorRef{{ID: "v2", Name: "Amazon Web Services", DefaultAccount: "6200"}}
		},
	}
	ai := &fakeAI{
		extract: `{"vendor_name":"Amazon","total_cents":1080,"line_items":[],"confidence":0.95}`,
		vendor:  `{"vendor_id":"v2","is_new_vendor":false,"proposed_vendor":null,"confidence":0.95,"reason":"x"}`,
	}
	res, err := Run(context.Background(), ai, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called != "Amazon" {
		t.Fatalf("Candidates callback should be invoked with the extracted vendor name, got %q", called)
	}
	// Matched candidate carries the default account → classify skipped, account resolved.
	if ai.classifyCalls != 0 || res.Classify == nil || res.Classify.AccountCode != "6200" {
		t.Fatalf("callback candidate's default account should resolve + skip classify: %+v", res.Classify)
	}
}

func TestRun_ProposesNewVendor_WhenNoCandidates(t *testing.T) {
	in := baseInput()
	in.Vendors = nil // no candidate vendors at all
	ai := &fakeAI{
		extract:  `{"vendor_name":"SQ *BLUE BOTTLE","total_cents":500,"line_items":[],"confidence":0.95}`,
		vendor:   `{"vendor_id":null,"is_new_vendor":true,"proposed_vendor":{"name":"Blue Bottle Coffee","match_pattern":"BLUE BOTTLE","tax_id":null,"website":null},"confidence":0.9,"reason":"cleaned"}`,
		classify: `{"account_code":"6200","confidence":0.9,"reason":"coffee"}`,
	}
	res, err := Run(context.Background(), ai, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ai.vendorCalls != 1 {
		t.Fatalf("vendor-match should run even with no candidates (to propose), got %d", ai.vendorCalls)
	}
	if res.VendorID != nil {
		t.Fatalf("a new vendor must have nil VendorID, got %v", *res.VendorID)
	}
	if res.ProposedVendor == nil || res.ProposedVendor.Name != "Blue Bottle Coffee" || res.ProposedVendor.MatchPattern != "BLUE BOTTLE" {
		t.Fatalf("expected a proposed vendor with clean name + pattern, got %+v", res.ProposedVendor)
	}
	if res.Status != "ready" {
		t.Fatalf("pipeline should still build a draft for a new vendor, got %+v", res)
	}
}

func TestRun_ParksOnLowExtractConfidence(t *testing.T) {
	ai := &fakeAI{extract: `{"vendor_name":"AWS","total_cents":1080,"line_items":[],"confidence":0.3}`}
	res, _ := Run(context.Background(), ai, baseInput())
	if res.Status != "needs_review" || res.FailedStage != "extract" {
		t.Fatalf("expected park at extract, got %+v", res)
	}
	if res.Classify != nil || res.Entry != nil {
		t.Fatal("must not proceed past a parked stage")
	}
}

func TestRun_ParksOnOffListAccount(t *testing.T) {
	ai := &fakeAI{
		extract:  `{"vendor_name":"Amazon Web Services","total_cents":1080,"line_items":[],"confidence":0.95}`,
		classify: `{"account_code":"9999","confidence":0.99,"reason":"nope"}`,
	}
	res, _ := Run(context.Background(), ai, baseInput())
	if res.Status != "needs_review" || res.FailedStage != "classify-account" {
		t.Fatalf("off-list account should park at classify, got %+v", res)
	}
	if res.Entry != nil {
		t.Fatal("no entry should be built when classification is parked")
	}
}

func TestRun_ParksWhenEntryCannotBuild(t *testing.T) {
	in := baseInput()
	in.CreditAccount = "" // build-entry can't fund the credit leg
	ai := &fakeAI{
		extract:  `{"vendor_name":"Amazon Web Services","total_cents":1080,"line_items":[],"confidence":0.95}`,
		classify: `{"account_code":"6200","confidence":0.9,"reason":"cloud"}`,
	}
	res, _ := Run(context.Background(), ai, in)
	if res.Status != "needs_review" || res.FailedStage != "build-entry" {
		t.Fatalf("expected park at build-entry, got %+v", res)
	}
}

func TestRun_PropagatesTransportError(t *testing.T) {
	ai := &fakeAI{err: errors.New("429 rate limited")}
	if _, err := Run(context.Background(), ai, baseInput()); err == nil {
		t.Fatal("transport error should propagate (job retries)")
	}
}
