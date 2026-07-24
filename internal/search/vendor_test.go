package search

import (
	"context"
	"strings"
	"testing"
)

func TestSearchDocumentFromVendor(t *testing.T) {
	t.Parallel()

	doc := SearchDocumentFromVendor("tenant-1", VendorData{
		ID:               "v1",
		EntityID:         "e1",
		Name:             "Blue Bottle Coffee",
		MatchPattern:     "BLUE BOTTLE",
		Website:          "bluebottlecoffee.com",
		DefaultAccountID: "acct-meals",
	})

	if doc.ID != "vendor_v1" || doc.Kind != "vendor" || doc.ObjectID != "v1" {
		t.Fatalf("identity fields wrong: %+v", doc)
	}
	if doc.TenantID != "tenant-1" || doc.EntityID != "e1" {
		t.Fatalf("scope not carried: %+v", doc)
	}
	if doc.AccountID != "acct-meals" {
		t.Fatalf("default account should ride along for candidate use, got %q", doc.AccountID)
	}
	if doc.Title != "Blue Bottle Coffee" || doc.Subtitle != "BLUE BOTTLE" {
		t.Fatalf("display fields wrong: title=%q subtitle=%q", doc.Title, doc.Subtitle)
	}
	// Body must carry the learned match pattern so a messy receipt string retrieves this vendor.
	if !strings.Contains(strings.ToLower(doc.Body), "blue bottle") {
		t.Fatalf("body should include the match pattern for retrieval, got %q", doc.Body)
	}
	if doc.Href != "/vendors" || doc.Status != "active" {
		t.Fatalf("nav/status wrong: %+v", doc)
	}
}

func TestReindexDocuments_IndexesVendors(t *testing.T) {
	t.Parallel()

	provider := &fakeReindexProvider{}
	source := fakeReindexSource{
		entities: []EntityData{{ID: "e1", TenantID: "t1"}},
		vendors: map[string][]VendorData{
			"e1": {{ID: "v1", EntityID: "e1", Name: "Acme", MatchPattern: "ACME", DefaultAccountID: "a1"}},
		},
	}
	result, err := Reindexer{Provider: provider, Source: source}.ReindexDocuments(context.Background(), ReindexOptions{})
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.VendorCount != 1 {
		t.Fatalf("expected 1 vendor counted, got %d", result.VendorCount)
	}
	found := false
	for _, d := range provider.searchDocs {
		if d.Kind == "vendor" && d.ObjectID == "v1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("vendor document was not indexed for global search: %+v", provider.searchDocs)
	}
	// Also indexed into the dedicated _vendors retrieval collection.
	if len(provider.vendorDocs) != 1 || provider.vendorDocs[0].ID != "v1" || provider.vendorDocs[0].TenantID != "t1" {
		t.Fatalf("vendor was not indexed into the _vendors collection: %+v", provider.vendorDocs)
	}
}
