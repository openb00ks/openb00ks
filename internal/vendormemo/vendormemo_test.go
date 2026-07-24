package vendormemo

import (
	"context"
	"testing"

	"github.com/openb00ks/openb00ks/internal/pipeline"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

// These hermetic tests cover the fail-open guards — the guarantee that with no store/search/tenant the
// memoization layer degrades gracefully (no panic, correct sentinel) rather than erroring. The paths that
// actually touch Postgres + Typesense are exercised by the db/http integration tests.

func TestCandidates_NilStore(t *testing.T) {
	// A zero Deps (no store, no search) → no candidates, no panic.
	if got := (Deps{}).Candidates(context.Background(), "", "e1", "Amazon"); len(got) != 0 {
		t.Fatalf("nil store should yield no candidates, got %v", got)
	}
}

func TestMemoize_NilAndEmptyAreNoops(t *testing.T) {
	d := Deps{} // nil store
	if _, ok := d.Memoize(context.Background(), "", "e1", nil, "6200"); ok {
		t.Fatal("nil store / nil proposal must not report success")
	}
	if _, ok := d.Memoize(context.Background(), "", "e1", &pipeline.ProposedVendor{Name: "  "}, "6200"); ok {
		t.Fatal("blank-name proposal must not report success")
	}
	// Non-blank proposal but nil store → still false (can't create), no panic.
	if _, ok := d.Memoize(context.Background(), "", "e1", &pipeline.ProposedVendor{Name: "Blue Bottle"}, "6200"); ok {
		t.Fatal("nil store must not report success")
	}
}

func TestRecordResolutionAndLearnAccount_NilSafe(t *testing.T) {
	d := Deps{}
	// Must not panic with nothing wired.
	d.RecordResolution(context.Background(), "", "e1", "v1", "SQ *ACME")
	d.RecordResolution(context.Background(), "", "e1", "", "blank vendor id is a no-op")
	if err := d.LearnAccount(context.Background(), "", "e1", "v1", "SQ *ACME", "6200"); err != nil {
		t.Fatalf("LearnAccount with nil store should no-op, got %v", err)
	}
	if err := d.LearnAccount(context.Background(), "", "e1", "", "raw", ""); err != nil {
		t.Fatalf("blank vendor/account should no-op, got %v", err)
	}
}

func TestSearchRefs_SignalsFallbackWhenScopeMissing(t *testing.T) {
	// searchRefs returns nil (= "fall back to a full DB scan") unless it has a store, a provider, and a
	// tenant scope — the fail-open guarantee that search never gates retrieval.
	present := searchpkg.NoopProvider{}
	cases := []struct {
		name   string
		search searchpkg.Provider
		tenant string
	}{
		{"nil provider", nil, "t1"},
		{"provider but no tenant scope", present, ""},
	}
	for _, tc := range cases {
		d := Deps{Search: tc.search} // nil Vendors also forces fallback; the point is no panic + nil result
		if refs := d.searchRefs(context.Background(), tc.tenant, "e1", "Acme"); refs != nil {
			t.Fatalf("%s: expected fallback (nil), got %v", tc.name, refs)
		}
	}
}
