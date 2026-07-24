package receiptbatch

import (
	"testing"

	"github.com/openb00ks/openb00ks/internal/pipeline"
)

func TestMatchedJSON_RoundTrips(t *testing.T) {
	vm, err := pipeline.ParseVendorMatch(string(matchedJSON("v2")))
	if err != nil {
		t.Fatalf("matchedJSON must parse back: %v", err)
	}
	if vm.VendorID == nil || *vm.VendorID != "v2" || vm.Confidence != 1 {
		t.Fatalf("exact-match json should carry the id at full confidence, got %+v", vm)
	}
}
