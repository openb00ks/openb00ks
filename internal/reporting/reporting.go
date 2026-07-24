// Package reporting holds double-entry financial-statement domain logic:
// account-type normalization and the normal-balance sign convention shared by
// the report handlers and the tax-pack CSV writers, so the two can never
// disagree on how general-ledger accounts roll up into statements.
package reporting

import "strings"

// NormalizeType canonicalizes an account-type spelling to one of
// "asset", "liability", "equity", "income", "expense", or "other".
func NormalizeType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "asset", "assets":
		return "asset"
	case "liability", "liabilities":
		return "liability"
	case "equity":
		return "equity"
	case "income", "revenue":
		return "income"
	case "expense", "expenses":
		return "expense"
	default:
		return "other"
	}
}

// DebitNormal reports whether an account type carries its balance on the debit
// side (assets and expenses). Income, liabilities, equity, and unknown types
// are credit-normal.
func DebitNormal(accountType string) bool {
	switch NormalizeType(accountType) {
	case "asset", "expense":
		return true
	default:
		return false
	}
}

// NormalBalance returns an account's balance on its natural side, in cents,
// from period debit/credit totals: debit-normal accounts are debit - credit,
// credit-normal accounts are credit - debit.
func NormalBalance(accountType string, debitCents, creditCents int64) int64 {
	if DebitNormal(accountType) {
		return debitCents - creditCents
	}
	return creditCents - debitCents
}

// SplitDebitCredit places a signed net balance (debit minus credit) into the
// debit or credit column of a trial balance.
func SplitDebitCredit(net int64) (debit, credit int64) {
	if net > 0 {
		return net, 0
	}
	return 0, -net
}
