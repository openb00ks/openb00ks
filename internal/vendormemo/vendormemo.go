// Package vendormemo is the vendor-memoization domain: it turns raw receipt vendor strings into
// first-class vendors that get better at matching over time. Three write paths feed it — a new-vendor
// proposal (Memoize), a pipeline resolution (RecordResolution), and a reviewer's correction on post
// (LearnAccount) — and one read path (Candidates) retrieves the shortlist the AI adjudicates.
//
// It owns the coupling between Postgres (the vendors table + vendor_aliases ledger + per-vendor default
// account) and Typesense (the _vendors retrieval collection + _documents global-search index). Search is
// always optional and fail-open: every method degrades to the DB when no provider is configured, so
// turning Typesense off never changes correctness.
package vendormemo

import (
	"context"
	"strings"

	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/pipeline"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

// candidateLimit caps the vendor shortlist offered to the model / retrieved from search.
const candidateLimit = 8

// Deps bundles the stores + the optional search provider the memoization layer needs. A zero Search means
// DB-only behaviour. Construct one and call its methods; the same Deps is safe to reuse and share.
type Deps struct {
	Vendors *db.VendorStore
	Aliases *db.VendorAliasStore
	Search  searchpkg.Provider
}

// Candidates returns the ranked vendor shortlist for a raw receipt vendor string. When a search provider
// is configured it retrieves a typo-tolerant shortlist from the _vendors collection (scales past a full
// table scan and matches on learned aliases, not just the display name); it always falls back to ranking
// over every vendor in the DB when search is absent, errors, or returns nothing — so search never gates
// correctness.
func (d Deps) Candidates(ctx context.Context, tenantID, entityID, vendorName string) []pipeline.VendorRef {
	// Search hits are already relevance-ranked (aliases included) and capped, so they're returned as-is —
	// re-ranking them through RankVendorCandidates would drop a vendor matched only via an alias. The DB
	// fallback, in contrast, is the full unranked pool and needs ranking.
	if refs := d.searchRefs(ctx, tenantID, entityID, vendorName); refs != nil {
		return refs
	}
	refs := make([]pipeline.VendorRef, 0)
	for _, v := range d.listVendors(ctx, entityID) {
		refs = append(refs, refFromDB(v))
	}
	return pipeline.RankVendorCandidates(vendorName, refs, candidateLimit)
}

// Memoize upserts a first-class vendor from an AI proposal (clean name + pattern + tax/website) with
// accountID as its default, so future receipts match it and reuse the account, then indexes it for global
// search. Returns the created vendor and true, or false when there's no usable proposal. Callers then
// RecordResolution to log the raw alias.
func (d Deps) Memoize(ctx context.Context, tenantID, entityID string, pv *pipeline.ProposedVendor, accountID string) (db.Vendor, bool) {
	if d.Vendors == nil || pv == nil || strings.TrimSpace(pv.Name) == "" {
		return db.Vendor{}, false
	}
	v, err := d.Vendors.Create(ctx, db.Vendor{
		EntityID:         entityID,
		Name:             pv.Name,
		NormalizedName:   pipeline.NormalizeVendorName(pv.Name),
		MatchPattern:     pv.MatchPattern,
		TaxID:            derefStr(pv.TaxID),
		Website:          derefStr(pv.Website),
		DefaultAccountID: accountID,
	})
	if err != nil {
		return db.Vendor{}, false
	}
	d.indexGlobalSearch(ctx, tenantID, v)
	return v, true
}

// RecordResolution logs that rawVendor (the messy receipt string) resolved to vendorID — appending to the
// alias ledger, bumping the vendor's counters — then refreshes its _vendors retrieval document so the new
// alias is immediately matchable. Best-effort; called for both matched and newly-created vendors.
func (d Deps) RecordResolution(ctx context.Context, tenantID, entityID, vendorID, rawVendor string) {
	if strings.TrimSpace(vendorID) == "" {
		return
	}
	if d.Aliases != nil {
		_ = d.Aliases.Record(ctx, vendorID, entityID, rawVendor, pipeline.NormalizeVendorName(rawVendor))
	}
	d.IndexRetrieval(ctx, tenantID, vendorID)
}

// LearnAccount is the reviewer feedback loop: when a human posts a receipt draft, chosenAccount is the
// expense account they settled on — the highest-signal correction available. It overwrites the vendor's
// default account when it differs (so the next receipt from this vendor pre-fills the reviewer's choice,
// not the AI's), reinforces the alias for rawVendor, and refreshes the retrieval doc. Best-effort and
// idempotent; a blank vendorID/account is a no-op. Returns an error only on a hard DB failure.
func (d Deps) LearnAccount(ctx context.Context, tenantID, entityID, vendorID, rawVendor, chosenAccount string) error {
	if d.Vendors == nil || strings.TrimSpace(vendorID) == "" || strings.TrimSpace(chosenAccount) == "" {
		return nil
	}
	v, err := d.Vendors.GetByID(ctx, vendorID)
	if err != nil {
		return err
	}
	if v.DefaultAccountID != chosenAccount {
		if err := d.Vendors.SetDefaultAccount(ctx, vendorID, chosenAccount); err != nil {
			return err
		}
	}
	// Authoritative alias write: reassigns the raw string to this vendor, so a corrected vendor takes
	// over the string from the AI's wrong guess (a normal confirmation is a no-op — same vendor).
	if d.Aliases != nil && strings.TrimSpace(rawVendor) != "" {
		_ = d.Aliases.RecordConfirmed(ctx, vendorID, entityID, rawVendor, pipeline.NormalizeVendorName(rawVendor))
	}
	d.IndexRetrieval(ctx, tenantID, vendorID)
	return nil
}

// IndexRetrieval best-effort refreshes a vendor's _vendors (retrieval) document from the DB, including its
// current aliases + counters. No-op without a provider / tenant scope; a reindex backfills.
func (d Deps) IndexRetrieval(ctx context.Context, tenantID, vendorID string) {
	if d.Search == nil || tenantID == "" || d.Vendors == nil {
		return
	}
	v, err := d.Vendors.GetByID(ctx, vendorID)
	if err != nil {
		return
	}
	var aliases []string
	if d.Aliases != nil {
		aliases, _ = d.Aliases.ListNormalized(ctx, vendorID)
	}
	lastSeen := int64(0)
	if !v.LastSeen.IsZero() {
		lastSeen = v.LastSeen.Unix()
	}
	_ = d.Search.UpsertVendor(ctx, searchpkg.VendorDocumentFromData(tenantID, searchpkg.VendorData{
		ID: v.ID, EntityID: v.EntityID, Name: v.Name, MatchPattern: v.MatchPattern, TaxID: v.TaxID,
		Website: v.Website, DefaultAccountID: v.DefaultAccountID, Aliases: aliases,
		ReceiptCount: int32(v.ReceiptCount), LastSeenUnix: lastSeen,
	}))
}

// searchRefs retrieves candidate vendors from the dedicated _vendors collection, building refs straight
// from the hits (the collection carries the full payload — no DB round-trip). It leads with the
// deterministic exact-normalized match (search can lag a just-created vendor or miss on typos). Returns
// nil to signal "fall back to a full scan" (no search, scope missing, error, or no hits).
func (d Deps) searchRefs(ctx context.Context, tenantID, entityID, vendorName string) []pipeline.VendorRef {
	if d.Vendors == nil || d.Search == nil || tenantID == "" {
		return nil
	}
	hits, err := d.Search.SearchVendors(ctx, searchpkg.VendorQuery{
		TenantID: tenantID, EntityID: entityID, Query: vendorName, Limit: candidateLimit,
	})
	if err != nil || len(hits) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	refs := make([]pipeline.VendorRef, 0, len(hits)+1)
	if v, ok, ferr := d.Vendors.FindByNormalized(ctx, entityID, pipeline.NormalizeVendorName(vendorName)); ferr == nil && ok {
		seen[v.ID] = struct{}{}
		refs = append(refs, refFromDB(v))
	}
	for _, h := range hits {
		doc := h.Document
		if doc.ID == "" {
			continue
		}
		if _, dup := seen[doc.ID]; dup {
			continue
		}
		seen[doc.ID] = struct{}{}
		refs = append(refs, pipeline.VendorRef{
			ID: doc.ID, Name: doc.Name, TaxID: doc.TaxID, MatchPattern: doc.MatchPattern, DefaultAccount: doc.DefaultAccountID,
		})
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

// indexGlobalSearch best-effort upserts a vendor's _documents (human global search) doc — distinct from
// IndexRetrieval, which serves the machine matching path.
func (d Deps) indexGlobalSearch(ctx context.Context, tenantID string, v db.Vendor) {
	if d.Search == nil || tenantID == "" {
		return
	}
	_ = d.Search.UpsertDocument(ctx, searchpkg.SearchDocumentFromVendor(tenantID, searchpkg.VendorData{
		ID: v.ID, EntityID: v.EntityID, Name: v.Name, MatchPattern: v.MatchPattern,
		Website: v.Website, DefaultAccountID: v.DefaultAccountID,
	}))
}

func (d Deps) listVendors(ctx context.Context, entityID string) []db.Vendor {
	if d.Vendors == nil {
		return nil
	}
	list, err := d.Vendors.ListForEntity(ctx, entityID, 500)
	if err != nil {
		return nil
	}
	return list
}

func refFromDB(v db.Vendor) pipeline.VendorRef {
	return pipeline.VendorRef{
		ID: v.ID, Name: v.Name, TaxID: v.TaxID, MatchPattern: v.MatchPattern, DefaultAccount: v.DefaultAccountID,
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
