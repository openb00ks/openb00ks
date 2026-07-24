package httpapi

import "testing"

// allocationChecklist is the shared source for both tax-checklist renderers, so
// its label / ok / count mapping is worth pinning.
func TestAllocationChecklist(t *testing.T) {
	t.Parallel()
	pct := 30
	profile := taxUseProfileState{HomeUtilitiesBusinessUsePercent: &pct} // cell + internet unset

	rows := allocationChecklist(profile)
	if len(rows) != 3 {
		t.Fatalf("want 3 allocation rows, got %d", len(rows))
	}

	// Utilities is set: ok, and allocateCount reports 0 missing.
	if rows[0].label != "Home utilities allocation" || !rows[0].ok || rows[0].count != 0 {
		t.Errorf("utilities row = %+v", rows[0])
	}
	// Cell phone is unset: not ok, count 1 (missing).
	if rows[1].label != "Cell phone allocation" || rows[1].ok || rows[1].count != 1 {
		t.Errorf("cell phone row = %+v", rows[1])
	}
	// Internet is unset: not ok.
	if rows[2].label != "Home internet allocation" || rows[2].ok {
		t.Errorf("internet row = %+v", rows[2])
	}
}
