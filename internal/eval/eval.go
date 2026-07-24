// Package eval is the receipt-pipeline accuracy harness. It replays labeled receipts through the SAME
// pipeline the worker runs (internal/pipeline.Run) and scores each stage against gold, so a prompt or
// model change surfaces per-stage accuracy deltas before it ships (see docs/receipt-pipeline.md). The
// scoring is pure; RunCase needs a pipeline.Completer (a real provider in cmd/receipt-bench, a fake in
// tests).
package eval

import (
	"context"

	"github.com/openb00ks/openb00ks/internal/pipeline"
)

// Case is one labeled receipt: the pipeline inputs plus the gold outcome.
type Case struct {
	Name          string                `json:"name"`
	OCRText       string                `json:"ocr_text"`
	Accounts      []pipeline.AccountRef `json:"accounts"`
	Vendors       []pipeline.VendorRef  `json:"vendors"`
	CreditAccount string                `json:"credit_account"`
	TaxAccount    string                `json:"tax_account"`

	// Gold. Pointer/empty fields are "don't score this dimension for this case".
	WantTotalCents  *int64 `json:"want_total_cents"`
	WantVendorName  string `json:"want_vendor_name"`
	WantAccountCode string `json:"want_account_code"`
	WantReady       *bool  `json:"want_ready"`
}

// CaseScore is the per-dimension verdict for one case. A dimension is nil when the case has no gold
// for it (so it doesn't count for or against accuracy).
type CaseScore struct {
	Name        string
	Total       *bool // extract total matched
	Account     *bool // classified account matched
	Vendor      *bool // vendor resolved/proposed as expected
	Ready       *bool // ready/parked matched expectation
	FailedStage string
	Err         error
}

// ScoreCase compares a pipeline result to the gold. Pure.
func ScoreCase(c Case, res pipeline.RunResult) CaseScore {
	s := CaseScore{Name: c.Name, FailedStage: res.FailedStage}

	if c.WantTotalCents != nil {
		got := res.Extract.TotalCents
		ok := got != nil && *got == *c.WantTotalCents
		s.Total = &ok
	}
	if c.WantAccountCode != "" {
		ok := res.Classify != nil && res.Classify.AccountCode == c.WantAccountCode
		s.Account = &ok
	}
	if c.WantVendorName != "" {
		ok := vendorMatches(c.WantVendorName, res, c.Vendors)
		s.Vendor = &ok
	}
	if c.WantReady != nil {
		ok := (res.Status == "ready") == *c.WantReady
		s.Ready = &ok
	}
	return s
}

// vendorMatches is true when the resolved vendor (a matched candidate's name, or the proposed
// new-vendor name) equals the gold vendor under the pipeline's normalization.
func vendorMatches(want string, res pipeline.RunResult, candidates []pipeline.VendorRef) bool {
	wn := pipeline.NormalizeVendorName(want)
	if res.ProposedVendor != nil && pipeline.NormalizeVendorName(res.ProposedVendor.Name) == wn {
		return true
	}
	if res.VendorID != nil {
		for _, v := range candidates {
			if v.ID == *res.VendorID && pipeline.NormalizeVendorName(v.Name) == wn {
				return true
			}
		}
	}
	// Extracted vendor name as a fallback signal.
	if res.Extract.VendorName != nil && pipeline.NormalizeVendorName(*res.Extract.VendorName) == wn {
		return true
	}
	return false
}

// RunCase runs one case through the pipeline and scores it.
func RunCase(ctx context.Context, ai pipeline.Completer, c Case) (pipeline.RunResult, CaseScore, error) {
	res, err := pipeline.Run(ctx, ai, pipeline.RunInput{
		OCRText:       c.OCRText,
		Accounts:      c.Accounts,
		Vendors:       c.Vendors,
		CreditAccount: c.CreditAccount,
		TaxAccount:    c.TaxAccount,
	})
	if err != nil {
		return res, CaseScore{Name: c.Name, Err: err}, err
	}
	return res, ScoreCase(c, res), nil
}

// Report aggregates scores across cases into per-dimension accuracy.
type Report struct {
	Cases   int
	Scores  []CaseScore
	Total   Accuracy
	Account Accuracy
	Vendor  Accuracy
	Ready   Accuracy
	Errors  int
}

// Accuracy is correct/total for a scored dimension.
type Accuracy struct {
	Correct int
	Scored  int
}

func (a Accuracy) Pct() float64 {
	if a.Scored == 0 {
		return 0
	}
	return float64(a.Correct) / float64(a.Scored) * 100
}

// Aggregate rolls per-case scores into a Report.
func Aggregate(scores []CaseScore) Report {
	r := Report{Cases: len(scores), Scores: scores}
	tally := func(acc *Accuracy, v *bool) {
		if v == nil {
			return
		}
		acc.Scored++
		if *v {
			acc.Correct++
		}
	}
	for _, s := range scores {
		if s.Err != nil {
			r.Errors++
			continue
		}
		tally(&r.Total, s.Total)
		tally(&r.Account, s.Account)
		tally(&r.Vendor, s.Vendor)
		tally(&r.Ready, s.Ready)
	}
	return r
}
