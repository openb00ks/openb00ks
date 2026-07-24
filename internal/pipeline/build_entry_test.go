package pipeline

import "testing"

func sumLegs(e DraftEntry) (debits, credits int64) {
	for _, l := range e.Lines {
		debits += l.DebitCents
		credits += l.CreditCents
	}
	return
}

func TestBuildEntry_SimpleBalanced(t *testing.T) {
	in := BuildEntryInput{
		Extract:        ExtractResult{VendorName: str("Staples"), Date: str("2026-07-19"), TotalCents: i64(1080)},
		ExpenseAccount: "6000",
		CreditAccount:  "2000", // accounts payable / clearing
	}
	e, err := BuildEntry(in)
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	d, c := sumLegs(e)
	if d != c || d != 1080 {
		t.Fatalf("not balanced to total: debits=%d credits=%d", d, c)
	}
	if len(e.Lines) != 2 || e.Lines[0].AccountCode != "6000" || e.Lines[0].DebitCents != 1080 {
		t.Fatalf("expense leg wrong: %+v", e.Lines)
	}
	if e.Memo == "" {
		t.Fatalf("memo should be set from vendor/date")
	}
}

func TestBuildEntry_TaxSplit(t *testing.T) {
	in := BuildEntryInput{
		Extract:        ExtractResult{TotalCents: i64(1080), SubtotalCents: i64(1000), TaxCents: i64(80)},
		ExpenseAccount: "6000",
		CreditAccount:  "2000",
		TaxAccount:     "1400", // sales-tax receivable
	}
	e, err := BuildEntry(in)
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if len(e.Lines) != 3 {
		t.Fatalf("expected 3 legs (expense, tax, credit), got %d", len(e.Lines))
	}
	d, c := sumLegs(e)
	if d != c || d != 1080 {
		t.Fatalf("tax-split entry not balanced: debits=%d credits=%d", d, c)
	}
}

func TestBuildEntry_Rejections(t *testing.T) {
	base := BuildEntryInput{ExpenseAccount: "6000", CreditAccount: "2000"}

	if _, err := BuildEntry(base); err == nil {
		t.Fatal("missing total should error")
	}
	noAcct := BuildEntryInput{Extract: ExtractResult{TotalCents: i64(100)}}
	if _, err := BuildEntry(noAcct); err == nil {
		t.Fatal("missing accounts should error")
	}
	neg := BuildEntryInput{Extract: ExtractResult{TotalCents: i64(-5)}, ExpenseAccount: "6000", CreditAccount: "2000"}
	if _, err := BuildEntry(neg); err == nil {
		t.Fatal("non-positive total should error")
	}
	// Tax split that doesn't reconcile to the total is refused.
	badTax := BuildEntryInput{
		Extract:        ExtractResult{TotalCents: i64(1080), SubtotalCents: i64(1000), TaxCents: i64(200)},
		ExpenseAccount: "6000", CreditAccount: "2000", TaxAccount: "1400",
	}
	if _, err := BuildEntry(badTax); err == nil {
		t.Fatal("subtotal+tax != total should error")
	}
}

func TestAssertBalanced(t *testing.T) {
	unbalanced := DraftEntry{TotalCents: 100, Lines: []EntryLine{
		{AccountCode: "6000", DebitCents: 100},
		{AccountCode: "2000", CreditCents: 90},
	}}
	if err := AssertBalanced(unbalanced); err == nil {
		t.Fatal("AssertBalanced must catch debits != credits")
	}
}
