package receiptbatch

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/openb00ks/openb00ks/internal/aibatch"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/pipeline"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/vendormemo"
	aipkg "github.com/spectrum-labs-tech/ai"
)

var zeroTemp = 0.0

func det() aipkg.Options { return aipkg.Options{Temperature: &zeroTemp} }

// Deps are the stores the receipt batch kinds need. BatchLimit caps receipts gathered per submit.
type Deps struct {
	States        *db.ReceiptPipelineStateStore
	Receipts      *db.ReceiptStore
	OCR           *db.ReceiptOCRStore
	Accounts      *db.AccountStore
	Drafts        *db.DraftStore
	Vendors       *db.VendorStore
	VendorAliases *db.VendorAliasStore // memoization ledger (optional)
	Entities      *db.EntityStore      // resolves an entity's tenant for search scoping (optional)
	Search        searchpkg.Provider   // optional augmentation for vendor retrieval + indexing (fail-open)
	BatchLimit    int
}

func (d Deps) limit() int {
	if d.BatchLimit <= 0 {
		return 200
	}
	return d.BatchLimit
}

// Kinds returns the three receipt AI stages as aibatch kinds, ready to register.
func Kinds(d Deps) []aibatch.Kind {
	return []aibatch.Kind{extractKind{d}, vendorKind{d}, classifyKind{d}}
}

// ── extract ──────────────────────────────────────────────────────────────────
type extractKind struct{ d Deps }

func (extractKind) Name() string { return KindExtract }
func (k extractKind) Reset(ctx context.Context, ids []string) error {
	return k.d.States.Reset(ctx, ids)
}

func (k extractKind) Gather(ctx context.Context) ([]aibatch.Item, error) {
	states, err := k.d.States.ClaimPending(ctx, StageExtract, k.d.limit())
	if err != nil {
		return nil, err
	}
	var items []aibatch.Item
	for _, st := range states {
		text := ""
		if latest, lerr := k.d.OCR.LatestByReceiptID(ctx, st.ReceiptID); lerr == nil {
			text = latest.RawText
		}
		if strings.TrimSpace(text) == "" {
			_ = k.d.States.Park(ctx, st.ReceiptID, StageExtract, "no OCR text")
			continue
		}
		sys, user, schema := pipeline.BuildExtractRequest(text)
		items = append(items, aibatch.Item{RefID: st.ReceiptID, Request: aipkg.BatchRequest{
			SystemPrompt: sys, UserPrompt: user, JSONSchema: schema, Options: det(),
		}})
	}
	return items, nil
}

func (k extractKind) Apply(ctx context.Context, refID, result string, resultErr error) error {
	if resultErr != nil {
		return k.d.States.Park(ctx, refID, StageExtract, resultErr.Error())
	}
	j, ok, issues := decideExtract(result)
	if !ok {
		return k.d.States.Park(ctx, refID, StageExtract, strings.Join(issues, "; "))
	}
	return k.d.States.SaveExtractAndAdvance(ctx, refID, j, NextStage(StageExtract))
}

// ── vendor-match ─────────────────────────────────────────────────────────────
type vendorKind struct{ d Deps }

func (vendorKind) Name() string                                    { return KindVendor }
func (k vendorKind) Reset(ctx context.Context, ids []string) error { return k.d.States.Reset(ctx, ids) }

func (k vendorKind) Gather(ctx context.Context) ([]aibatch.Item, error) {
	states, err := k.d.States.ClaimPending(ctx, StageVendor, k.d.limit())
	if err != nil {
		return nil, err
	}
	var items []aibatch.Item
	for _, st := range states {
		ex, perr := pipeline.ParseExtract(string(st.ExtractJSON))
		if perr != nil || ex.VendorName == nil || strings.TrimSpace(*ex.VendorName) == "" {
			_ = k.d.States.SaveVendorAndAdvance(ctx, st.ReceiptID, nil, NextStage(StageVendor))
			continue
		}
		cands := k.candidatesForReceipt(ctx, st.ReceiptID, *ex.VendorName)
		// Deterministic exact match short-circuits the AI (still advances to classify).
		if id := pipeline.ExactVendorMatch(*ex.VendorName, cands); id != nil {
			_ = k.d.States.SaveVendorAndAdvance(ctx, st.ReceiptID, matchedJSON(*id), NextStage(StageVendor))
			continue
		}
		sys, user, schema := pipeline.BuildVendorMatchRequest(*ex.VendorName, cands)
		items = append(items, aibatch.Item{RefID: st.ReceiptID, Request: aipkg.BatchRequest{
			SystemPrompt: sys, UserPrompt: user, JSONSchema: schema, Options: det(),
		}})
	}
	return items, nil
}

func (k vendorKind) Apply(ctx context.Context, refID, result string, resultErr error) error {
	if resultErr != nil {
		return k.d.States.Park(ctx, refID, StageVendor, resultErr.Error())
	}
	cands := k.candidatesForRef(ctx, refID)
	j, ok, issues := decideVendor(result, cands)
	if !ok {
		return k.d.States.Park(ctx, refID, StageVendor, strings.Join(issues, "; "))
	}
	return k.d.States.SaveVendorAndAdvance(ctx, refID, j, NextStage(StageVendor))
}

// candidatesForRef re-derives the vendor shortlist for a receipt from its state (deterministic, so it
// matches what Gather offered — needed to validate an AI match in Apply).
func (k vendorKind) candidatesForRef(ctx context.Context, receiptID string) []pipeline.VendorRef {
	st, err := k.d.States.Get(ctx, receiptID)
	if err != nil {
		return nil
	}
	ex, perr := pipeline.ParseExtract(string(st.ExtractJSON))
	if perr != nil || ex.VendorName == nil {
		return nil
	}
	return k.candidatesForReceipt(ctx, receiptID, *ex.VendorName)
}

func (k vendorKind) candidatesForReceipt(ctx context.Context, receiptID, vendorName string) []pipeline.VendorRef {
	receipt, err := k.d.Receipts.GetByID(ctx, receiptID)
	if err != nil {
		return nil
	}
	return k.d.memo().Candidates(ctx, k.d.tenantFor(ctx, receipt.EntityID), receipt.EntityID, vendorName)
}

// ── classify-account (terminal: builds the draft) ────────────────────────────
type classifyKind struct{ d Deps }

func (classifyKind) Name() string { return KindClassify }
func (k classifyKind) Reset(ctx context.Context, ids []string) error {
	return k.d.States.Reset(ctx, ids)
}

func (k classifyKind) Gather(ctx context.Context) ([]aibatch.Item, error) {
	states, err := k.d.States.ClaimPending(ctx, StageClassify, k.d.limit())
	if err != nil {
		return nil, err
	}
	var items []aibatch.Item
	for _, st := range states {
		ex, perr := pipeline.ParseExtract(string(st.ExtractJSON))
		if perr != nil {
			_ = k.d.States.Park(ctx, st.ReceiptID, StageClassify, "unreadable extract state")
			continue
		}
		receipt, rerr := k.d.Receipts.GetByID(ctx, st.ReceiptID)
		if rerr != nil {
			_ = k.d.States.Reset(ctx, []string{st.ReceiptID})
			continue
		}
		// Memoization: a matched vendor with a default account skips the classify AI call.
		if acct := k.vendorDefaultAccount(ctx, receipt.EntityID, st.VendorJSON); acct != "" {
			_ = finalize(ctx, k.d, st.ReceiptID, receipt, ex, st.VendorJSON, acct, 1, "vendor default account")
			continue
		}
		accts := accountRefs(ctx, k.d.Accounts, receipt.EntityID)
		sys, user, schema := pipeline.BuildClassifyRequest(ex, accts)
		items = append(items, aibatch.Item{RefID: st.ReceiptID, Request: aipkg.BatchRequest{
			SystemPrompt: sys, UserPrompt: user, JSONSchema: schema, Options: det(),
		}})
	}
	return items, nil
}

func (k classifyKind) Apply(ctx context.Context, refID, result string, resultErr error) error {
	if resultErr != nil {
		return k.d.States.Park(ctx, refID, StageClassify, resultErr.Error())
	}
	st, err := k.d.States.Get(ctx, refID)
	if err != nil {
		return err
	}
	ex, perr := pipeline.ParseExtract(string(st.ExtractJSON))
	if perr != nil {
		return k.d.States.Park(ctx, refID, StageClassify, "unreadable extract state")
	}
	receipt, rerr := k.d.Receipts.GetByID(ctx, refID)
	if rerr != nil {
		return rerr
	}
	cl, cperr := pipeline.ParseClassify(result)
	if cperr != nil {
		return k.d.States.Park(ctx, refID, StageClassify, cperr.Error())
	}
	if out := pipeline.GateClassify(cl, accountRefs(ctx, k.d.Accounts, receipt.EntityID)); !out.Advance {
		return k.d.States.Park(ctx, refID, StageClassify, strings.Join(out.Issues, "; "))
	}
	return finalize(ctx, k.d, refID, receipt, ex, st.VendorJSON, cl.AccountCode, cl.Confidence, cl.Reason)
}

// vendorDefaultAccount returns the default account of the vendor matched in vendorJSON, if any.
func (k classifyKind) vendorDefaultAccount(ctx context.Context, _ string, vendorJSON []byte) string {
	if k.d.Vendors == nil || len(vendorJSON) == 0 {
		return ""
	}
	vm, err := pipeline.ParseVendorMatch(string(vendorJSON))
	if err != nil || vm.VendorID == nil {
		return ""
	}
	v, err := k.d.Vendors.GetByID(ctx, *vm.VendorID)
	if err != nil {
		return ""
	}
	return v.DefaultAccountID
}

// finalize is the terminal step: build a balanced draft (deterministic), memoize a proposed vendor,
// persist a display summary, and mark the receipt ready for review — or park on a build failure.
// accountConf/accountReason describe how the account was chosen (an AI classify result, or the matched
// vendor's default when classify was skipped).
func finalize(ctx context.Context, d Deps, receiptID string, receipt models.Receipt, ex pipeline.ExtractResult, vendorJSON []byte, accountCode string, accountConf float64, accountReason string) error {
	credit := ""
	if cash, cerr := d.Accounts.FindDefaultCashAccount(ctx, receipt.EntityID); cerr == nil {
		credit = cash.ID
	}
	entry, berr := pipeline.BuildEntry(pipeline.BuildEntryInput{
		Extract: ex, ExpenseAccount: accountCode, CreditAccount: credit,
	})
	if berr != nil {
		return d.States.Park(ctx, receiptID, "build-entry", berr.Error())
	}

	entries := make([]models.DraftEntry, 0, len(entry.Lines))
	for _, l := range entry.Lines {
		entries = append(entries, models.DraftEntry{AccountID: l.AccountCode, DebitCents: l.DebitCents, CreditCents: l.CreditCents})
	}
	date := time.Now().UTC()
	if entry.Date != "" {
		if dparsed, derr := time.Parse("2006-01-02", entry.Date); derr == nil {
			date = dparsed
		}
	}
	if d.Drafts != nil {
		if _, eerr := d.Drafts.EnsureForReceipt(ctx, receiptID); eerr != nil {
			return eerr
		}
		if _, uerr := d.Drafts.UpdateDraft(ctx, receiptID, date, entry.Memo, entries); uerr != nil {
			return d.States.Park(ctx, receiptID, "build-entry", uerr.Error())
		}
	}
	_ = d.Receipts.UpdateStatus(ctx, receiptID, "ready_for_review")
	d.memoizeResolution(ctx, receiptID, receipt.EntityID, ex, vendorJSON, accountCode)
	if summary := d.buildSummary(ctx, vendorJSON, accountCode, accountConf, accountReason); summary.HasContent() {
		_ = d.Receipts.SetAISummary(ctx, receiptID, &summary)
	}
	return d.States.SaveClassifyAndFinish(ctx, receiptID, mustJSON(map[string]string{"account_code": accountCode}))
}

// buildSummary distills the batch stage outputs into the review-UI display summary.
func (d Deps) buildSummary(ctx context.Context, vendorJSON []byte, accountCode string, accountConf float64, accountReason string) models.ReceiptAISummary {
	var s models.ReceiptAISummary
	if vm, err := pipeline.ParseVendorMatch(string(vendorJSON)); err == nil {
		switch {
		case vm.ProposedVendor != nil:
			s.Vendor = &models.AIVendorSummary{Name: vm.ProposedVendor.Name, Confidence: vm.Confidence, Reason: vm.Reason, IsNew: true}
		case vm.VendorID != nil && d.Vendors != nil:
			name := ""
			if v, gerr := d.Vendors.GetByID(ctx, *vm.VendorID); gerr == nil {
				name = v.Name
			}
			reason := vm.Reason
			if reason == "" {
				reason = "exact name match"
			}
			s.Vendor = &models.AIVendorSummary{Name: name, Confidence: vm.Confidence, Reason: reason}
		}
	}
	if accountCode != "" {
		s.Account = &models.AIAccountSummary{AccountID: accountCode, Confidence: accountConf, Reason: accountReason}
	}
	return s
}

// memoizeResolution records the raw vendor string against the resolved vendor: for a proposal it creates
// the vendor first; then it logs the alias + refreshes the retrieval doc + persists the resolution on the
// receipt (so posting the draft can feed the reviewer's account choice back). Drives everything from the
// stage's vendor_json, so it covers exact match, AI match, and new-vendor alike. Best-effort.
func (d Deps) memoizeResolution(ctx context.Context, receiptID, entityID string, ex pipeline.ExtractResult, vendorJSON []byte, accountCode string) {
	raw := ""
	if ex.VendorName != nil {
		raw = strings.TrimSpace(*ex.VendorName)
	}
	vm, err := pipeline.ParseVendorMatch(string(vendorJSON))
	if err != nil {
		return
	}
	tenant := d.tenantFor(ctx, entityID)
	m := d.memo()
	resolvedVendorID := ""
	switch {
	case vm.ProposedVendor != nil:
		if v, ok := m.Memoize(ctx, tenant, entityID, vm.ProposedVendor, accountCode); ok {
			m.RecordResolution(ctx, tenant, entityID, v.ID, raw)
			resolvedVendorID = v.ID
		}
	case vm.VendorID != nil:
		m.RecordResolution(ctx, tenant, entityID, *vm.VendorID, raw)
		resolvedVendorID = *vm.VendorID
	}
	if resolvedVendorID != "" && d.Receipts != nil {
		_ = d.Receipts.SetResolvedVendor(ctx, receiptID, resolvedVendorID, raw)
	}
}

// tenantFor resolves an entity's tenant for search scoping, or "" when unavailable (search then no-ops).
func (d Deps) tenantFor(ctx context.Context, entityID string) string {
	if d.Entities == nil {
		return ""
	}
	e, err := d.Entities.GetByID(ctx, entityID)
	if err != nil {
		return ""
	}
	return e.TenantID
}

// memo is the vendor-memoization domain, wired from this Deps' stores.
func (d Deps) memo() vendormemo.Deps {
	return vendormemo.Deps{Vendors: d.Vendors, Aliases: d.VendorAliases, Search: d.Search}
}

func accountRefs(ctx context.Context, store *db.AccountStore, entityID string) []pipeline.AccountRef {
	var refs []pipeline.AccountRef
	if store == nil {
		return refs
	}
	list, err := store.ListForEntity(ctx, entityID, 500)
	if err != nil {
		return refs
	}
	for _, a := range list {
		refs = append(refs, pipeline.AccountRef{Code: a.ID, Name: a.Name, Type: a.Type})
	}
	return refs
}

// matchedJSON is the vendor_json for a deterministic exact match (no AI call).
func matchedJSON(vendorID string) []byte {
	return mustJSON(pipeline.VendorMatchResult{VendorID: &vendorID, Confidence: 1})
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// decideExtract parses + gates an extract result. Pure.
func decideExtract(raw string) ([]byte, bool, []string) {
	r, err := pipeline.ParseExtract(raw)
	if err != nil {
		return nil, false, []string{err.Error()}
	}
	out := pipeline.GateExtract(r)
	return mustJSON(r), out.Advance, out.Issues
}

// decideVendor parses + gates a vendor-match result against the offered candidates. Pure.
func decideVendor(raw string, candidates []pipeline.VendorRef) ([]byte, bool, []string) {
	r, err := pipeline.ParseVendorMatch(raw)
	if err != nil {
		return nil, false, []string{err.Error()}
	}
	out := pipeline.GateVendorMatch(r, candidates)
	return mustJSON(r), out.Advance, out.Issues
}
