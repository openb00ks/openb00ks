package pipeline

import "testing"

var testVendors = []VendorRef{
	{ID: "v1", Name: "Staples, Inc.", TaxID: "12-3456789"},
	{ID: "v2", Name: "Amazon Web Services"},
}

func TestNormalizeVendorName(t *testing.T) {
	if got := NormalizeVendorName("Staples, Inc."); got != "staples" {
		t.Fatalf("normalize = %q, want staples", got)
	}
	if NormalizeVendorName("ACME LLC") != "acme" {
		t.Fatalf("suffix not stripped")
	}
}

func TestExactVendorMatch(t *testing.T) {
	if id := ExactVendorMatch("STAPLES INC", testVendors); id == nil || *id != "v1" {
		t.Fatalf("expected exact match to v1, got %v", id)
	}
	if id := ExactVendorMatch("Costco Wholesale", testVendors); id != nil {
		t.Fatalf("expected no exact match, got %v", *id)
	}
}

func TestBuildVendorMatchRequest(t *testing.T) {
	sys, user, schema := BuildVendorMatchRequest("AWS", testVendors)
	if !contains(sys, "id=v1") || !contains(sys, "tax_id=12-3456789") || !contains(sys, "id=v2") {
		t.Fatalf("candidates not injected: %s", sys)
	}
	if !contains(sys, "CONSERVATIVE") {
		t.Fatalf("missing conservatism instruction")
	}
	if !contains(user, "AWS") || schema != VendorSchema {
		t.Fatalf("bad user/schema")
	}
}

func TestValidateAndGateVendorMatch(t *testing.T) {
	// Confident match to an offered id → advance.
	m := VendorMatchResult{VendorID: str("v2"), Confidence: 0.92}
	if out := GateVendorMatch(m, testVendors); !out.Advance {
		t.Fatalf("confident offered match should advance, got %+v", out)
	}
	// Confident new vendor WITH a proposal → advance (orchestration creates it).
	nv := VendorMatchResult{IsNewVendor: true, Confidence: 0.9,
		ProposedVendor: &ProposedVendor{Name: "Blue Bottle Coffee", MatchPattern: "BLUE BOTTLE"}}
	if out := GateVendorMatch(nv, testVendors); !out.Advance {
		t.Fatalf("confident new-vendor with a proposal should advance, got %+v", out)
	}
	// New vendor missing the proposal (or its name/pattern) → flagged, can't create a vendor from it.
	if len(ValidateVendorMatch(VendorMatchResult{IsNewVendor: true, Confidence: 0.9}, testVendors)) == 0 {
		t.Fatal("is_new_vendor without proposed_vendor should be flagged")
	}
	if len(ValidateVendorMatch(VendorMatchResult{IsNewVendor: true, Confidence: 0.9,
		ProposedVendor: &ProposedVendor{Name: "X"}}, testVendors)) == 0 {
		t.Fatal("proposed_vendor without match_pattern should be flagged")
	}
	// id not in candidate list → reject.
	bad := VendorMatchResult{VendorID: str("v9"), Confidence: 0.99}
	if len(ValidateVendorMatch(bad, testVendors)) == 0 || GateVendorMatch(bad, testVendors).Advance {
		t.Fatal("off-list vendor_id must be rejected")
	}
	// id set AND is_new_vendor true → inconsistent.
	if len(ValidateVendorMatch(VendorMatchResult{VendorID: str("v1"), IsNewVendor: true, Confidence: 0.9}, testVendors)) == 0 {
		t.Fatal("id+is_new_vendor should be flagged inconsistent")
	}
	// Undecided (no id, not new) → flagged.
	if len(ValidateVendorMatch(VendorMatchResult{Confidence: 0.9}, testVendors)) == 0 {
		t.Fatal("undecided result should be flagged")
	}
	// Low confidence → review.
	if GateVendorMatch(VendorMatchResult{VendorID: str("v2"), Confidence: 0.5}, testVendors).Advance {
		t.Fatal("low-confidence match must not advance")
	}
}

func TestRankVendorCandidates(t *testing.T) {
	cands := []VendorRef{
		{ID: "v1", Name: "Blue Bottle Coffee"},
		{ID: "v2", Name: "Amazon Web Services"},
		{ID: "v3", Name: "Bluebird Cafe"},
	}
	got := RankVendorCandidates("SQ *BLUE BOTTLE", cands, 8)
	if len(got) == 0 || got[0].ID != "v1" {
		t.Fatalf("expected Blue Bottle Coffee ranked first, got %+v", got)
	}
	for _, c := range got {
		if c.ID == "v2" {
			t.Fatal("an unrelated vendor should not be a candidate")
		}
	}
	if l := RankVendorCandidates("Blue", cands, 1); len(l) > 1 {
		t.Fatalf("limit not honored: %d candidates", len(l))
	}
	if exact := RankVendorCandidates("Amazon Web Services", cands, 8); len(exact) == 0 || exact[0].ID != "v2" {
		t.Fatalf("exact normalized match should rank first: %+v", exact)
	}
	if RankVendorCandidates("", cands, 8) != nil {
		t.Fatal("empty query yields no candidates")
	}
}

func TestRankVendorCandidates_UsesMatchPattern(t *testing.T) {
	// A vendor whose DISPLAY name barely resembles the messy receipt string still ranks first via its
	// learned match_pattern — this is the memoization payoff (re-matching a vendor's own future receipts).
	cands := []VendorRef{
		{ID: "v1", Name: "Starbucks Corporation"},
		{ID: "v2", Name: "The Roastery LLC", MatchPattern: "BLUE BOTTLE"},
	}
	got := RankVendorCandidates("SQ *BLUE BOTTLE #4471", cands, 8)
	if len(got) == 0 || got[0].ID != "v2" {
		t.Fatalf("match_pattern should surface The Roastery first, got %+v", got)
	}
	// Without the pattern the same vendor would not be retrievable at all.
	noPattern := []VendorRef{{ID: "v2", Name: "The Roastery LLC"}}
	if r := RankVendorCandidates("SQ *BLUE BOTTLE #4471", noPattern, 8); len(r) != 0 {
		t.Fatalf("display name alone should not match this receipt string, got %+v", r)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
