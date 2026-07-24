package receiptbatch

import "testing"

func TestDecideExtract(t *testing.T) {
	// Valid + balanced + confident → advance.
	j, ok, issues := decideExtract(`{"vendor_name":"ACME","date":"2026-07-19","currency":"USD","subtotal_cents":1000,"tax_cents":80,"total_cents":1080,"line_items":[],"confidence":0.95}`)
	if !ok || len(issues) != 0 || len(j) == 0 {
		t.Fatalf("clean extract should advance: ok=%v issues=%v", ok, issues)
	}
	// Broken arithmetic at high confidence → park.
	if _, ok, _ := decideExtract(`{"subtotal_cents":1000,"tax_cents":80,"total_cents":9999,"line_items":[],"confidence":0.99}`); ok {
		t.Fatal("broken arithmetic must not advance")
	}
	// Invalid JSON → park with an issue.
	if _, ok, issues := decideExtract("nonsense"); ok || len(issues) == 0 {
		t.Fatal("invalid JSON should park with an issue")
	}
}

func TestDecideVendor(t *testing.T) {
	// Confident new-vendor proposal → advance.
	if _, ok, _ := decideVendor(`{"vendor_id":null,"is_new_vendor":true,"proposed_vendor":{"name":"Blue Bottle","match_pattern":"BLUE BOTTLE","tax_id":null,"website":null},"confidence":0.9,"reason":"x"}`, nil); !ok {
		t.Fatal("confident proposal should advance")
	}
	// New vendor without a usable proposal → park.
	if _, ok, _ := decideVendor(`{"vendor_id":null,"is_new_vendor":true,"proposed_vendor":null,"confidence":0.9,"reason":"x"}`, nil); ok {
		t.Fatal("new vendor without a proposal must not advance")
	}
	// Invalid JSON → park.
	if _, ok, _ := decideVendor("{", nil); ok {
		t.Fatal("invalid JSON should park")
	}
}
