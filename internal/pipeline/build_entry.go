package pipeline

import (
	"errors"
	"fmt"
	"strings"
)

// EntryLine is one leg of a double-entry journal entry (exactly one of debit/credit is non-zero).
type EntryLine struct {
	AccountCode string
	DebitCents  int64
	CreditCents int64
}

// DraftEntry is a balanced journal entry ready to become a draft transaction for human approval.
type DraftEntry struct {
	Date       string
	Memo       string
	Lines      []EntryLine
	TotalCents int64
}

// BuildEntryInput carries the classified/extracted facts plus the funding + optional tax accounts the
// orchestration supplies (the receipt itself doesn't name the account it was paid from).
type BuildEntryInput struct {
	Extract        ExtractResult
	ExpenseAccount string // from classify-account
	CreditAccount  string // the funding source (bank / credit card / accounts payable / clearing)
	TaxAccount     string // optional: when set + tax present, tax is split onto its own line
}

// BuildEntry assembles a balanced journal entry deterministically (NO model). Debits fund the expense
// (and tax, if split); the credit is the funding account. It is also the final validator: it refuses
// to emit anything that isn't balanced.
func BuildEntry(in BuildEntryInput) (DraftEntry, error) {
	if in.Extract.TotalCents == nil {
		return DraftEntry{}, errors.New("build-entry: receipt has no total")
	}
	if in.ExpenseAccount == "" || in.CreditAccount == "" {
		return DraftEntry{}, errors.New("build-entry: expense and credit accounts are required")
	}
	total := *in.Extract.TotalCents
	if total <= 0 {
		return DraftEntry{}, fmt.Errorf("build-entry: non-positive total %d", total)
	}

	var lines []EntryLine
	if in.TaxAccount != "" && in.Extract.TaxCents != nil && in.Extract.SubtotalCents != nil {
		sub, tax := *in.Extract.SubtotalCents, *in.Extract.TaxCents
		if abs64(sub+tax-total) > centsTolerance {
			return DraftEntry{}, fmt.Errorf("build-entry: subtotal %d + tax %d != total %d", sub, tax, total)
		}
		lines = []EntryLine{
			{AccountCode: in.ExpenseAccount, DebitCents: sub},
			{AccountCode: in.TaxAccount, DebitCents: tax},
			{AccountCode: in.CreditAccount, CreditCents: total},
		}
	} else {
		lines = []EntryLine{
			{AccountCode: in.ExpenseAccount, DebitCents: total},
			{AccountCode: in.CreditAccount, CreditCents: total},
		}
	}

	entry := DraftEntry{
		Date:       derefStr(in.Extract.Date),
		Memo:       buildMemo(in.Extract),
		Lines:      lines,
		TotalCents: total,
	}
	if err := AssertBalanced(entry); err != nil {
		return DraftEntry{}, err
	}
	return entry, nil
}

// AssertBalanced enforces the double-entry invariant: Σdebits == Σcredits == total. The pipeline must
// never post an unbalanced entry — this is the deterministic "free correctness check".
func AssertBalanced(e DraftEntry) error {
	var debits, credits int64
	for _, l := range e.Lines {
		if l.DebitCents < 0 || l.CreditCents < 0 {
			return fmt.Errorf("build-entry: negative amount on account %s", l.AccountCode)
		}
		debits += l.DebitCents
		credits += l.CreditCents
	}
	if debits != credits {
		return fmt.Errorf("build-entry: unbalanced (debits %d != credits %d)", debits, credits)
	}
	if debits != e.TotalCents {
		return fmt.Errorf("build-entry: entry total %d != debit total %d", e.TotalCents, debits)
	}
	return nil
}

func buildMemo(r ExtractResult) string {
	var parts []string
	if r.VendorName != nil && *r.VendorName != "" {
		parts = append(parts, *r.VendorName)
	}
	if r.Date != nil && *r.Date != "" {
		parts = append(parts, *r.Date)
	}
	return strings.Join(parts, " — ")
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
