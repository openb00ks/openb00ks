package models

// ReceiptAISummary is a compact, display-oriented record of what the AI pipeline decided for a receipt —
// persisted so the review UI can explain the suggested vendor + account (name, confidence, reason) to the
// reviewer before they approve. Stored as JSONB on the receipt; nil stages are omitted.
type ReceiptAISummary struct {
	Vendor  *AIVendorSummary  `json:"vendor,omitempty"`
	Account *AIAccountSummary `json:"account,omitempty"`
}

// AIVendorSummary explains the vendor-match stage. IsNew marks a vendor the pipeline recommended creating
// (vs. matched an existing one).
type AIVendorSummary struct {
	Name       string  `json:"name,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	IsNew      bool    `json:"is_new,omitempty"`
}

// AIAccountSummary explains the classify-account stage. AccountID is the entity GL account the pipeline
// chose; the UI resolves it to a name from the chart of accounts.
type AIAccountSummary struct {
	AccountID  string  `json:"account_id,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

// HasContent reports whether the summary carries anything worth showing.
func (s ReceiptAISummary) HasContent() bool {
	return s.Vendor != nil || s.Account != nil
}
