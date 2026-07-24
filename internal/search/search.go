package search

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrScopeRequired = errors.New("tenant_id and entity_id are required")

type Provider interface {
	SearchTransactions(ctx context.Context, query TransactionQuery) ([]TransactionMatch, error)
	SearchDocuments(ctx context.Context, query DocumentQuery) ([]DocumentMatch, error)
	SuggestCandidates(ctx context.Context, query CandidateQuery) ([]Candidate, error)
	SearchVendors(ctx context.Context, query VendorQuery) ([]VendorMatch, error)
	UpsertTransaction(ctx context.Context, doc TransactionDocument) error
	UpsertDocument(ctx context.Context, doc SearchDocument) error
	UpsertVendor(ctx context.Context, doc VendorDocument) error
	DeleteDocument(ctx context.Context, id string) error
	DeleteVendor(ctx context.Context, id string) error
}

type TransactionQuery struct {
	TenantID string
	EntityID string
	Query    string
	Limit    int
}

type DocumentQuery struct {
	TenantID   string
	EntityID   string
	Query      string
	Kinds      []string
	Statuses   []string
	AccountIDs []string
	Tags       []string
	StartDate  string
	EndDate    string
	Limit      int
}

type CandidateQuery struct {
	TenantID    string
	EntityID    string
	Query       string
	AmountCents int64
	Limit       int
}

type TransactionDocument struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	EntityID        string    `json:"entity_id"`
	TransactionID   string    `json:"transaction_id"`
	Date            string    `json:"date"`
	DateUnix        int64     `json:"date_unix"`
	Memo            string    `json:"memo"`
	Description     string    `json:"description"`
	AccountIDs      []string  `json:"account_ids"`
	AccountNames    []string  `json:"account_names"`
	AccountRoleTags []string  `json:"account_role_tags"`
	AmountCents     int64     `json:"amount_cents"`
	Source          string    `json:"source"`
	PostedAt        time.Time `json:"posted_at"`
}

type TransactionMatch struct {
	Document TransactionDocument `json:"document"`
	Score    float64             `json:"score"`
}

type SearchDocument struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	EntityID    string    `json:"entity_id"`
	Kind        string    `json:"kind"`
	ObjectID    string    `json:"object_id"`
	AccountID   string    `json:"account_id"`
	AccountName string    `json:"account_name"`
	Title       string    `json:"title"`
	Subtitle    string    `json:"subtitle"`
	Body        string    `json:"body"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	Date        string    `json:"date"`
	DateUnix    int64     `json:"date_unix"`
	AmountCents int64     `json:"amount_cents"`
	Href        string    `json:"href"`
	IndexedAt   time.Time `json:"indexed_at"`
}

type DocumentMatch struct {
	Document SearchDocument `json:"document"`
	Score    float64        `json:"score"`
}

// VendorQuery searches the dedicated _vendors collection for pipeline candidate retrieval (distinct from
// the polymorphic _documents used for human global search).
type VendorQuery struct {
	TenantID string
	EntityID string
	Query    string
	Limit    int
}

// VendorDocument is a vendor as indexed for retrieval. It carries the full matching payload (aliases,
// pattern, default account, tax id) so a hit needs no DB round-trip. json tags mirror the collection.
type VendorDocument struct {
	ID               string   `json:"id"`
	TenantID         string   `json:"tenant_id"`
	EntityID         string   `json:"entity_id"`
	Name             string   `json:"name"`
	Aliases          []string `json:"aliases"`
	MatchPattern     string   `json:"match_pattern"`
	TaxID            string   `json:"tax_id"`
	Website          string   `json:"website"`
	DefaultAccountID string   `json:"default_account_id"`
	ReceiptCount     int32    `json:"receipt_count"`
	LastSeenUnix     int64    `json:"last_seen_unix"`
}

type VendorMatch struct {
	Document VendorDocument `json:"document"`
	Score    float64        `json:"score"`
}

type Candidate struct {
	TransactionID   string   `json:"transaction_id"`
	AccountID       string   `json:"account_id"`
	AccountName     string   `json:"account_name"`
	AccountRoleTags []string `json:"account_role_tags"`
	Memo            string   `json:"memo"`
	Description     string   `json:"description"`
	AmountCents     int64    `json:"amount_cents"`
	Date            string   `json:"date"`
	Score           float64  `json:"score"`
}

type NoopProvider struct{}

func (NoopProvider) SearchTransactions(context.Context, TransactionQuery) ([]TransactionMatch, error) {
	return []TransactionMatch{}, nil
}

func (NoopProvider) SearchDocuments(context.Context, DocumentQuery) ([]DocumentMatch, error) {
	return []DocumentMatch{}, nil
}

func (NoopProvider) SuggestCandidates(context.Context, CandidateQuery) ([]Candidate, error) {
	return []Candidate{}, nil
}

func (NoopProvider) SearchVendors(context.Context, VendorQuery) ([]VendorMatch, error) {
	return []VendorMatch{}, nil
}

func (NoopProvider) UpsertTransaction(context.Context, TransactionDocument) error {
	return nil
}

func (NoopProvider) UpsertDocument(context.Context, SearchDocument) error {
	return nil
}

func (NoopProvider) UpsertVendor(context.Context, VendorDocument) error {
	return nil
}

func (NoopProvider) DeleteDocument(context.Context, string) error {
	return nil
}

func (NoopProvider) DeleteVendor(context.Context, string) error {
	return nil
}

func NormalizeQuery(parts ...string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return strings.Join(out, " ")
}

func BestCandidate(candidates []Candidate, minScore float64) (Candidate, bool) {
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Score > best.Score {
			best = candidate
		}
	}
	if strings.TrimSpace(best.AccountID) == "" || best.Score < minScore {
		return Candidate{}, false
	}
	return best, true
}

func TransactionDocumentFromData(tenantID string, transactionID string, entityID string, date time.Time, memo string, entries []EntryData, accounts []AccountData, createdAt time.Time) TransactionDocument {
	accountIDs := make([]string, 0, len(entries))
	seenAccounts := map[string]struct{}{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.AccountID) == "" {
			continue
		}
		if _, ok := seenAccounts[entry.AccountID]; ok {
			continue
		}
		seenAccounts[entry.AccountID] = struct{}{}
		accountIDs = append(accountIDs, entry.AccountID)
	}
	nameByID := map[string]string{}
	tagsByID := map[string][]string{}
	for _, account := range accounts {
		nameByID[account.ID] = account.Name
		tagsByID[account.ID] = account.RoleTags
	}
	accountNames := make([]string, 0, len(accountIDs))
	accountRoleTags := []string{}
	seenTags := map[string]struct{}{}
	for _, accountID := range accountIDs {
		if name := nameByID[accountID]; name != "" {
			accountNames = append(accountNames, name)
		}
		for _, tag := range tagsByID[accountID] {
			if strings.TrimSpace(tag) == "" {
				continue
			}
			if _, ok := seenTags[tag]; ok {
				continue
			}
			seenTags[tag] = struct{}{}
			accountRoleTags = append(accountRoleTags, tag)
		}
	}
	amount := int64(0)
	for _, entry := range entries {
		amount += entry.DebitCents
	}
	return TransactionDocument{
		ID:              transactionID,
		TenantID:        tenantID,
		EntityID:        entityID,
		TransactionID:   transactionID,
		Date:            date.Format("2006-01-02"),
		DateUnix:        date.Unix(),
		Memo:            memo,
		Description:     NormalizeQuery(memo, strings.Join(accountNames, " "), strings.Join(accountRoleTags, " ")),
		AccountIDs:      accountIDs,
		AccountNames:    accountNames,
		AccountRoleTags: accountRoleTags,
		AmountCents:     amount,
		Source:          "transaction",
		PostedAt:        createdAt,
	}
}

func SearchDocumentFromTransaction(doc TransactionDocument) SearchDocument {
	dateUnix := doc.DateUnix
	if dateUnix == 0 {
		if parsed, err := time.Parse("2006-01-02", doc.Date); err == nil {
			dateUnix = parsed.Unix()
		}
	}
	indexedAt := doc.PostedAt
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}
	return SearchDocument{
		ID:          "transaction_" + doc.TransactionID,
		TenantID:    doc.TenantID,
		EntityID:    doc.EntityID,
		Kind:        "transaction",
		ObjectID:    doc.TransactionID,
		AccountID:   firstString(doc.AccountIDs),
		AccountName: firstString(doc.AccountNames),
		Title:       firstNonEmpty(doc.Memo, "Transaction"),
		Subtitle:    strings.Join(doc.AccountNames, ", "),
		Body:        NormalizeQuery(doc.Memo, doc.Description, strings.Join(doc.AccountNames, " "), strings.Join(doc.AccountRoleTags, " ")),
		Status:      "posted",
		Tags:        doc.AccountRoleTags,
		Date:        doc.Date,
		DateUnix:    dateUnix,
		AmountCents: doc.AmountCents,
		Href:        "/transactions",
		IndexedAt:   indexedAt,
	}
}

func SearchDocumentFromReceipt(tenantID string, receipt ReceiptData) SearchDocument {
	kind := receipt.Kind
	if strings.TrimSpace(kind) == "" {
		kind = "receipt"
	}
	title := firstNonEmpty(receipt.OriginalName, kind)
	indexedAt := time.Now().UTC()
	dateUnix := int64(0)
	date := ""
	if !receipt.UploadedAt.IsZero() {
		indexedAt = receipt.UploadedAt
		date = receipt.UploadedAt.Format("2006-01-02")
		dateUnix = receipt.UploadedAt.Unix()
	}
	href := "/receipts/" + receipt.ID
	if kind == "import" {
		href = "/imports/" + receipt.ID
	}
	return SearchDocument{
		ID:          kind + "_" + receipt.ID,
		TenantID:    tenantID,
		EntityID:    receipt.EntityID,
		Kind:        kind,
		ObjectID:    receipt.ID,
		Title:       title,
		Subtitle:    firstNonEmpty(strings.Join(receipt.TagNames, ", "), receipt.Status),
		Body:        NormalizeQuery(title, receipt.ContentType, receipt.Status, kind, strings.Join(receipt.TagNames, " ")),
		Status:      receipt.Status,
		Tags:        receipt.TagNames,
		Date:        date,
		DateUnix:    dateUnix,
		AmountCents: receipt.TotalCents,
		Href:        href,
		IndexedAt:   indexedAt,
	}
}

func SearchDocumentFromAccount(tenantID string, account AccountData) SearchDocument {
	indexedAt := account.CreatedAt
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}
	return SearchDocument{
		ID:          "account_" + account.ID,
		TenantID:    tenantID,
		EntityID:    account.EntityID,
		Kind:        "account",
		ObjectID:    account.ID,
		AccountID:   account.ID,
		AccountName: account.Name,
		Title:       firstNonEmpty(account.Name, "Account"),
		Subtitle:    account.Type,
		Body:        NormalizeQuery(account.Name, account.Type, strings.Join(account.RoleTags, " ")),
		Status:      "active",
		Tags:        account.RoleTags,
		Href:        "/accounts",
		IndexedAt:   indexedAt,
	}
}

func SearchDocumentFromVendor(tenantID string, vendor VendorData) SearchDocument {
	subtitle := vendor.MatchPattern
	if subtitle == "" {
		subtitle = vendor.Website
	}
	return SearchDocument{
		ID:        "vendor_" + vendor.ID,
		TenantID:  tenantID,
		EntityID:  vendor.EntityID,
		Kind:      "vendor",
		ObjectID:  vendor.ID,
		AccountID: vendor.DefaultAccountID,
		Title:     firstNonEmpty(vendor.Name, "Vendor"),
		Subtitle:  subtitle,
		// Body carries the messy signals a receipt string might contain (the display name AND the learned
		// match pattern) so Typesense's typo-tolerant search retrieves this vendor for future receipts.
		Body:      NormalizeQuery(vendor.Name, vendor.MatchPattern, vendor.Website),
		Status:    "active",
		Href:      "/vendors",
		IndexedAt: time.Now().UTC(),
	}
}

// VendorDocumentFromData builds the _vendors retrieval document. Aliases is normalized to a non-nil
// slice so Typesense receives [] rather than null for the string[] field.
func VendorDocumentFromData(tenantID string, vendor VendorData) VendorDocument {
	aliases := vendor.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	return VendorDocument{
		ID:               vendor.ID,
		TenantID:         tenantID,
		EntityID:         vendor.EntityID,
		Name:             vendor.Name,
		Aliases:          aliases,
		MatchPattern:     vendor.MatchPattern,
		TaxID:            vendor.TaxID,
		Website:          vendor.Website,
		DefaultAccountID: vendor.DefaultAccountID,
		ReceiptCount:     vendor.ReceiptCount,
		LastSeenUnix:     vendor.LastSeenUnix,
	}
}

func SearchDocumentFromStatement(tenantID string, statement StatementData) SearchDocument {
	indexedAt := statement.UpdatedAt
	if indexedAt.IsZero() {
		indexedAt = statement.CreatedAt
	}
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}
	date := ""
	dateUnix := int64(0)
	if !statement.PeriodEnd.IsZero() {
		date = statement.PeriodEnd.Format("2006-01-02")
		dateUnix = statement.PeriodEnd.Unix()
	}
	title := firstNonEmpty(statement.AccountName, "Account statement")
	if !statement.PeriodStart.IsZero() && !statement.PeriodEnd.IsZero() {
		title = title + " statement"
	}
	period := ""
	if !statement.PeriodStart.IsZero() && !statement.PeriodEnd.IsZero() {
		period = statement.PeriodStart.Format("2006-01-02") + " to " + statement.PeriodEnd.Format("2006-01-02")
	}
	return SearchDocument{
		ID:          "statement_" + statement.ID,
		TenantID:    tenantID,
		EntityID:    statement.EntityID,
		Kind:        "statement",
		ObjectID:    statement.ID,
		AccountID:   statement.AccountID,
		AccountName: statement.AccountName,
		Title:       title,
		Subtitle:    firstNonEmpty(period, statement.Status),
		Body:        NormalizeQuery(statement.AccountName, statement.AccountType, statement.SourceReceiptName, statement.Status, statement.Notes, period),
		Status:      statement.Status,
		Date:        date,
		DateUnix:    dateUnix,
		AmountCents: statement.EndingBalanceCents,
		Href:        "/statements",
		IndexedAt:   indexedAt,
	}
}

func SearchDocumentFromMileage(tenantID string, mileage MileageData) SearchDocument {
	indexedAt := mileage.UpdatedAt
	if indexedAt.IsZero() {
		indexedAt = mileage.CreatedAt
	}
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}
	date := ""
	dateUnix := int64(0)
	if !mileage.Date.IsZero() {
		date = mileage.Date.Format("2006-01-02")
		dateUnix = mileage.Date.Unix()
	}
	title := firstNonEmpty(mileage.Purpose, "Mileage")
	subtitleParts := []string{}
	if mileage.StartLocation != "" {
		subtitleParts = append(subtitleParts, mileage.StartLocation)
	}
	if mileage.EndLocation != "" {
		subtitleParts = append(subtitleParts, mileage.EndLocation)
	}
	return SearchDocument{
		ID:        "mileage_" + mileage.ID,
		TenantID:  tenantID,
		EntityID:  mileage.EntityID,
		Kind:      "mileage",
		ObjectID:  mileage.ID,
		Title:     title,
		Subtitle:  strings.Join(subtitleParts, " to "),
		Body:      NormalizeQuery(mileage.Purpose, mileage.StartLocation, mileage.EndLocation, mileage.SuggestionContext),
		Status:    "logged",
		Date:      date,
		DateUnix:  dateUnix,
		Href:      "/mileage",
		IndexedAt: indexedAt,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type EntryData struct {
	AccountID   string
	DebitCents  int64
	CreditCents int64
}

type AccountData struct {
	ID        string
	EntityID  string
	Name      string
	Type      string
	RoleTags  []string
	CreatedAt time.Time
}

type ReceiptData struct {
	ID           string
	EntityID     string
	Kind         string
	Status       string
	ContentType  string
	OriginalName string
	TotalCents   int64
	TagNames     []string
	UploadedAt   time.Time
}

type StatementData struct {
	ID                 string
	EntityID           string
	AccountID          string
	AccountName        string
	AccountType        string
	SourceReceiptName  string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	EndingBalanceCents int64
	Status             string
	Notes              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type VendorData struct {
	ID               string
	EntityID         string
	Name             string
	MatchPattern     string
	TaxID            string
	Website          string
	DefaultAccountID string
	Aliases          []string
	ReceiptCount     int32
	LastSeenUnix     int64
}

type MileageData struct {
	ID                string
	EntityID          string
	Date              time.Time
	DistanceMiles     float64
	StartLocation     string
	EndLocation       string
	Purpose           string
	SuggestionContext string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
