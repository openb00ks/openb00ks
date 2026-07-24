package pipeline

import "context"

// Completer runs one structured AI call. The worker adapts the shared ai.Provider to this (fixing
// temperature 0), keeping this package free of the AI-driver dependency and trivially fakeable.
type Completer interface {
	Complete(ctx context.Context, system, user, jsonSchema string) (string, error)
}

// RunInput is everything the pipeline needs for one receipt. Accounts + Vendors are pre-retrieved
// candidate sets (the models judge, they don't search). CreditAccount is the funding source and
// TaxAccount is optional.
type RunInput struct {
	OCRText  string
	Accounts []AccountRef
	// Vendors is a static candidate list. Candidates, if set, is called with the extracted vendor name
	// to retrieve candidates dynamically (used by the sync worker to query known vendors) and takes
	// precedence over Vendors.
	Vendors       []VendorRef
	Candidates    func(vendorName string) []VendorRef
	CreditAccount string
	TaxAccount    string
}

// RunResult is the pipeline outcome. Status is "ready" when every stage gated clean and a balanced
// entry was built, else "needs_review". FailedStage + Issues explain a park.
type RunResult struct {
	Status      string
	FailedStage string
	Issues      []string

	Extract        ExtractResult
	VendorMatch    *VendorMatchResult // nil when resolved by exact match or no vendor name
	VendorID       *string            // resolved vendor id (exact or AI match), nil for a new vendor
	ProposedVendor *ProposedVendor    // AI recommendation to create a vendor, when none matched
	Classify       *ClassifyResult    // nil if parked earlier
	Entry          *DraftEntry        // nil if parked before build
}

// defaultAccountFor returns the matched vendor candidate's default account, if any.
func defaultAccountFor(vendorID string, candidates []VendorRef) string {
	for _, c := range candidates {
		if c.ID == vendorID {
			return c.DefaultAccount
		}
	}
	return ""
}

func review(stage string, issues []string, partial RunResult) (RunResult, error) {
	partial.Status = "needs_review"
	partial.FailedStage = stage
	partial.Issues = issues
	return partial, nil
}

// Run executes the decomposed pipeline: extract → vendor-match → classify-account → build-entry.
// A gate failure at any stage parks the receipt (Status "needs_review") and stops — nothing is
// fabricated past a low-confidence or invalid stage. A transport error from the model is returned as
// an error (the caller retries the job).
func Run(ctx context.Context, ai Completer, in RunInput) (RunResult, error) {
	var res RunResult

	// Stage 2: extract.
	sys, user, schema := BuildExtractRequest(in.OCRText)
	raw, err := ai.Complete(ctx, sys, user, schema)
	if err != nil {
		return res, err
	}
	extract, err := ParseExtract(raw)
	if err != nil {
		return review("extract", []string{err.Error()}, res)
	}
	res.Extract = extract
	if out := GateExtract(extract); !out.Advance {
		return review("extract", out.Issues, res)
	}

	// Stage 3: vendor-match — retrieve candidates for the extracted vendor, deterministic exact pass,
	// else ask the AI to match a candidate OR recommend a new vendor (even with an empty candidate
	// list, so a brand-new vendor still gets a clean proposal).
	if extract.VendorName != nil && *extract.VendorName != "" {
		vname := *extract.VendorName
		candidates := in.Vendors
		if in.Candidates != nil {
			candidates = in.Candidates(vname)
		}
		if id := ExactVendorMatch(vname, candidates); id != nil {
			res.VendorID = id
		} else {
			vs, vu, vsch := BuildVendorMatchRequest(vname, candidates)
			vraw, verr := ai.Complete(ctx, vs, vu, vsch)
			if verr != nil {
				return res, verr
			}
			vm, perr := ParseVendorMatch(vraw)
			if perr != nil {
				return review("vendor-match", []string{perr.Error()}, res)
			}
			res.VendorMatch = &vm
			if out := GateVendorMatch(vm, candidates); !out.Advance {
				return review("vendor-match", out.Issues, res)
			}
			res.VendorID = vm.VendorID             // nil for a new vendor
			res.ProposedVendor = vm.ProposedVendor // set for a new vendor
		}
		// Memoization payoff: a matched vendor with a default account skips the classify AI call.
		if res.VendorID != nil {
			if acct := defaultAccountFor(*res.VendorID, candidates); acct != "" {
				res.Classify = &ClassifyResult{AccountCode: acct, Confidence: 1, Reason: "vendor default account"}
			}
		}
	}

	// Stage 4: classify-account (skipped when a matched vendor already set the account).
	if res.Classify == nil {
		cs, cu, csch := BuildClassifyRequest(extract, in.Accounts)
		craw, cerr := ai.Complete(ctx, cs, cu, csch)
		if cerr != nil {
			return res, cerr
		}
		cl, perr := ParseClassify(craw)
		if perr != nil {
			return review("classify-account", []string{perr.Error()}, res)
		}
		if out := GateClassify(cl, in.Accounts); !out.Advance {
			res.Classify = &cl
			return review("classify-account", out.Issues, res)
		}
		res.Classify = &cl
	}

	// Stage 5: build-entry (deterministic; the final validator).
	entry, berr := BuildEntry(BuildEntryInput{
		Extract:        extract,
		ExpenseAccount: res.Classify.AccountCode,
		CreditAccount:  in.CreditAccount,
		TaxAccount:     in.TaxAccount,
	})
	if berr != nil {
		return review("build-entry", []string{berr.Error()}, res)
	}
	res.Entry = &entry
	res.Status = "ready"
	return res, nil
}
