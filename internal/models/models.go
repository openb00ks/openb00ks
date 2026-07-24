package models

import (
	"encoding/json"
	"time"
)

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAccountant Role = "accountant"
	RoleUser       Role = "user"
)

const (
	AccountRoleUtilities = "utilities"
	AccountRoleCellPhone = "cell_phone"
	AccountRoleInternet  = "internet"
)

type User struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	PasswordHash    string    `json:"password_hash"`
	DefaultTenantID string    `json:"default_tenant_id,omitempty"`
	IsAdmin         bool      `json:"is_admin"`
	CreatedAt       time.Time `json:"created_at"`
}

type UserPreferences struct {
	UserID          string    `json:"user_id"`
	DefaultEntityID string    `json:"default_entity_id"`
	Theme           string    `json:"theme"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TenantID   string     `json:"tenant_id"`
	TokenHash  string     `json:"token_hash"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type MileageLog struct {
	ID            string    `json:"id"`
	EntityID      string    `json:"entity_id"`
	UserID        string    `json:"user_id"`
	ReceiptID     string    `json:"receipt_id"`
	Date          time.Time `json:"date"`
	DistanceMiles float64   `json:"distance_miles"`
	StartLocation string    `json:"start_location"`
	EndLocation   string    `json:"end_location"`
	Purpose       string    `json:"purpose"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MileageRate struct {
	Year             int       `json:"year"`
	RateCentsPerMile int       `json:"rate_cents_per_mile"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Entity struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	Name                 string    `json:"name"`
	SuggestionContext    string    `json:"suggestion_context"`
	FiscalYearStartMonth int       `json:"fiscal_year_start_month"`
	FiscalYearStartDay   int       `json:"fiscal_year_start_day"`
	CreatedAt            time.Time `json:"created_at"`
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type TenantMembership struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	UserID     string    `json:"user_id"`
	Role       Role      `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
}

type EntityUser struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	EntityID  string    `json:"entity_id"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Account struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Code      string    `json:"code,omitempty"`
	RoleTags  []string  `json:"role_tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AccountStatement struct {
	ID                         string    `json:"id"`
	EntityID                   string    `json:"entity_id"`
	AccountID                  string    `json:"account_id"`
	AccountName                string    `json:"account_name,omitempty"`
	AccountType                string    `json:"account_type,omitempty"`
	SourceReceiptID            string    `json:"source_receipt_id,omitempty"`
	SourceReceiptName          string    `json:"source_receipt_name,omitempty"`
	PeriodStart                time.Time `json:"period_start"`
	PeriodEnd                  time.Time `json:"period_end"`
	StartingBalanceCents       int64     `json:"starting_balance_cents"`
	EndingBalanceCents         int64     `json:"ending_balance_cents"`
	ImportedInflowCents        int64     `json:"imported_inflow_cents"`
	ImportedOutflowCents       int64     `json:"imported_outflow_cents"`
	PostedInflowCents          int64     `json:"posted_inflow_cents"`
	PostedOutflowCents         int64     `json:"posted_outflow_cents"`
	ExpectedEndingBalanceCents int64     `json:"expected_ending_balance_cents"`
	DifferenceCents            int64     `json:"difference_cents"`
	UnpostedRows               int       `json:"unposted_rows"`
	Status                     string    `json:"status"`
	Notes                      string    `json:"notes,omitempty"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type Receipt struct {
	ID           string     `json:"id"`
	EntityID     string     `json:"entity_id"`
	StorageKey   string     `json:"storage_key"`
	ContentType  string     `json:"content_type"`
	SizeBytes    int64      `json:"size_bytes"`
	Status       string     `json:"status"`
	Kind         string     `json:"kind"`
	TotalCents   int64      `json:"total_cents"`
	UploadedAt   time.Time  `json:"uploaded_at"`
	AttachedAt   *time.Time `json:"attached_at"`
	OriginalName string     `json:"original_name"`
	// ResolvedVendorID / ResolvedVendorRaw record the vendor the pipeline matched or created for this
	// receipt and the raw receipt string it matched — the substrate for the reviewer-feedback loop.
	ResolvedVendorID  string `json:"resolved_vendor_id,omitempty"`
	ResolvedVendorRaw string `json:"resolved_vendor_raw,omitempty"`
	// AISummary explains the pipeline's suggestion (vendor + account, with confidence/reason) for the
	// review UI. Nil for legacy-suggest or pre-summary receipts.
	AISummary *ReceiptAISummary `json:"ai_summary,omitempty"`
}

type ImportRow struct {
	ID            string          `json:"id"`
	ReceiptID     string          `json:"receipt_id"`
	EntityID      string          `json:"entity_id"`
	RowIndex      int             `json:"row_index"`
	Date          time.Time       `json:"date"`
	Vendor        string          `json:"vendor"`
	Memo          string          `json:"memo"`
	AmountCents   int64           `json:"amount_cents"`
	Direction     string          `json:"direction"`
	AccountID     string          `json:"account_id"`
	Fingerprint   string          `json:"fingerprint"`
	Status        string          `json:"status"`
	TransactionID string          `json:"transaction_id"`
	RawJSON       json.RawMessage `json:"raw_json"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Transaction struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	Date      time.Time `json:"date"`
	Memo      string    `json:"memo"`
	CreatedAt time.Time `json:"created_at"`
}

type Entry struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	AccountID     string `json:"account_id"`
	DebitCents    int64  `json:"debit_cents"`
	CreditCents   int64  `json:"credit_cents"`
}

type DraftTransaction struct {
	ID        string       `json:"id"`
	ReceiptID string       `json:"receipt_id"`
	EntityID  string       `json:"entity_id"`
	Date      time.Time    `json:"date"`
	Memo      string       `json:"memo"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Entries   []DraftEntry `json:"entries"`
}

type DraftEntry struct {
	ID                 string `json:"id"`
	DraftTransactionID string `json:"draft_transaction_id"`
	AccountID          string `json:"account_id"`
	DebitCents         int64  `json:"debit_cents"`
	CreditCents        int64  `json:"credit_cents"`
}

type VendorRule struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	MatchType string    `json:"match_type"`
	Pattern   string    `json:"pattern"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ReceiptOCR struct {
	ID         string          `json:"id"`
	ReceiptID  string          `json:"receipt_id"`
	Provider   string          `json:"provider"`
	Status     string          `json:"status"`
	RawText    string          `json:"raw_text"`
	DataJSON   json.RawMessage `json:"data_json"`
	Error      string          `json:"error"`
	InputHash  string          `json:"input_hash"`
	RunVersion int             `json:"run_version"`
	CreatedAt  time.Time       `json:"created_at"`
}

type ReceiptSuggestion struct {
	ID               string          `json:"id"`
	ReceiptID        string          `json:"receipt_id"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	Status           string          `json:"status"`
	PromptJSON       json.RawMessage `json:"prompt_json"`
	RawJSON          json.RawMessage `json:"raw_response"`
	ParsedJSON       json.RawMessage `json:"parsed_json"`
	Confidence       float64         `json:"confidence"`
	Error            string          `json:"error"`
	InputHash        string          `json:"input_hash"`
	RunVersion       int             `json:"run_version"`
	CreatedAt        time.Time       `json:"created_at"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalTokens      int64           `json:"total_tokens"`
	CostCents        int64           `json:"cost_cents"`
}

type ReceiptJob struct {
	ID          string     `json:"id"`
	ReceiptID   string     `json:"receipt_id"`
	Stage       string     `json:"stage"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	LockedUntil *time.Time `json:"locked_until"`
	LockedBy    string     `json:"locked_by"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ProcessingError struct {
	ID             string     `json:"id"`
	EntityID       string     `json:"entity_id"`
	ReceiptID      string     `json:"receipt_id"`
	MileageID      string     `json:"mileage_id"`
	Stage          string     `json:"stage"`
	Error          string     `json:"error"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolutionNote string     `json:"resolution_note"`
}

type Tag struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Suggestion status constants for receipt suggestions.
const (
	SuggestionStatusSucceeded     = "succeeded"
	SuggestionStatusSkipped       = "skipped"
	SuggestionStatusAIFailed      = "ai_failed"
	SuggestionStatusLimitExceeded = "limit_exceeded"
)
