package db

import (
	"database/sql"
	"time"
)

type UserRow struct {
	ID              string         `db:"id"`
	Email           string         `db:"email"`
	PasswordHash    string         `db:"password_hash"`
	DefaultTenantID sql.NullString `db:"default_tenant_id"`
	IsAdmin         bool           `db:"is_admin"`
	CreatedAt       time.Time      `db:"created_at"`
}

type EntityRow struct {
	ID                   string    `db:"id"`
	TenantID             string    `db:"tenant_id"`
	Name                 string    `db:"name"`
	SuggestionContext    string    `db:"suggestion_context"`
	FiscalYearStartMonth int       `db:"fiscal_year_start_month"`
	FiscalYearStartDay   int       `db:"fiscal_year_start_day"`
	CreatedAt            time.Time `db:"created_at"`
}

type EntityTaxSettingsRow struct {
	EntityID                       string        `db:"entity_id"`
	TaxYear                        int           `db:"tax_year"`
	HomeOfficeSqFt                 sql.NullInt64 `db:"home_office_sqft"`
	HomeTotalSqFt                  sql.NullInt64 `db:"home_total_sqft"`
	CellPhoneBusinessUsePercent    sql.NullInt64 `db:"cell_phone_business_use_percent"`
	HomeInternetBusinessUsePercent sql.NullInt64 `db:"home_internet_business_use_percent"`
	CreatedAt                      time.Time     `db:"created_at"`
	UpdatedAt                      time.Time     `db:"updated_at"`
}

type EntityUserRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	EntityID  string    `db:"entity_id"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}

type AccountRow struct {
	ID        string         `db:"id"`
	EntityID  string         `db:"entity_id"`
	Name      string         `db:"name"`
	Type      string         `db:"type"`
	Code      sql.NullString `db:"code"`
	CreatedAt time.Time      `db:"created_at"`
}

type AccountRoleTagRow struct {
	AccountID string    `db:"account_id"`
	RoleTag   string    `db:"role_tag"`
	CreatedAt time.Time `db:"created_at"`
}

type AccountStatementRow struct {
	ID                   string         `db:"id"`
	EntityID             string         `db:"entity_id"`
	AccountID            string         `db:"account_id"`
	AccountName          string         `db:"account_name"`
	AccountType          string         `db:"account_type"`
	SourceReceiptID      sql.NullString `db:"source_receipt_id"`
	SourceReceiptName    sql.NullString `db:"source_receipt_name"`
	PeriodStart          time.Time      `db:"period_start"`
	PeriodEnd            time.Time      `db:"period_end"`
	StartingBalanceCents int64          `db:"starting_balance_cents"`
	EndingBalanceCents   int64          `db:"ending_balance_cents"`
	Status               string         `db:"status"`
	Notes                sql.NullString `db:"notes"`
	CreatedAt            time.Time      `db:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at"`
	ImportedInflowCents  sql.NullInt64  `db:"imported_inflow_cents"`
	ImportedOutflowCents sql.NullInt64  `db:"imported_outflow_cents"`
	PostedInflowCents    sql.NullInt64  `db:"posted_inflow_cents"`
	PostedOutflowCents   sql.NullInt64  `db:"posted_outflow_cents"`
	UnpostedRows         sql.NullInt64  `db:"unposted_rows"`
}

type ReceiptRow struct {
	ID                string         `db:"id"`
	EntityID          string         `db:"entity_id"`
	StorageKey        string         `db:"storage_key"`
	ContentType       string         `db:"content_type"`
	SizeBytes         int64          `db:"size_bytes"`
	Status            string         `db:"status"`
	Kind              string         `db:"kind"`
	TotalCents        sql.NullInt64  `db:"total_cents"`
	UploadedAt        time.Time      `db:"uploaded_at"`
	AttachedAt        sql.NullTime   `db:"attached_at"`
	OriginalName      sql.NullString `db:"original_name"`
	ResolvedVendorID  sql.NullString `db:"resolved_vendor_id"`
	ResolvedVendorRaw sql.NullString `db:"resolved_vendor_raw"`
	AISummary         []byte         `db:"ai_summary"`
}

type ImportRowRow struct {
	ID            string         `db:"id"`
	ReceiptID     string         `db:"receipt_id"`
	EntityID      string         `db:"entity_id"`
	RowIndex      int            `db:"row_index"`
	Date          time.Time      `db:"date"`
	Vendor        string         `db:"vendor"`
	Memo          sql.NullString `db:"memo"`
	AmountCents   int64          `db:"amount_cents"`
	Direction     string         `db:"direction"`
	AccountID     sql.NullString `db:"account_id"`
	Fingerprint   string         `db:"fingerprint"`
	Status        string         `db:"status"`
	TransactionID sql.NullString `db:"transaction_id"`
	RawJSON       []byte         `db:"raw_json"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type TransactionRow struct {
	ID        string         `db:"id"`
	EntityID  string         `db:"entity_id"`
	Date      time.Time      `db:"date"`
	Memo      sql.NullString `db:"memo"`
	CreatedAt time.Time      `db:"created_at"`
}

type EntryRow struct {
	ID            string `db:"id"`
	TransactionID string `db:"transaction_id"`
	AccountID     string `db:"account_id"`
	DebitCents    int64  `db:"debit_cents"`
	CreditCents   int64  `db:"credit_cents"`
}

type UserPreferencesRow struct {
	UserID          string         `db:"user_id"`
	DefaultEntityID sql.NullString `db:"default_entity_id"`
	Theme           string         `db:"theme"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

type RefreshTokenRow struct {
	ID         string       `db:"id"`
	UserID     string       `db:"user_id"`
	TenantID   string       `db:"tenant_id"`
	TokenHash  string       `db:"token_hash"`
	ExpiresAt  time.Time    `db:"expires_at"`
	CreatedAt  time.Time    `db:"created_at"`
	LastUsedAt sql.NullTime `db:"last_used_at"`
	RevokedAt  sql.NullTime `db:"revoked_at"`
}

type TenantRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

type TenantMembershipRow struct {
	ID         string    `db:"id"`
	TenantID   string    `db:"tenant_id"`
	TenantName string    `db:"tenant_name"`
	UserID     string    `db:"user_id"`
	Role       string    `db:"role"`
	CreatedAt  time.Time `db:"created_at"`
}

type MileageLogRow struct {
	ID            string         `db:"id"`
	EntityID      string         `db:"entity_id"`
	UserID        sql.NullString `db:"user_id"`
	ReceiptID     sql.NullString `db:"receipt_id"`
	Date          time.Time      `db:"date"`
	DistanceMiles float64        `db:"distance_miles"`
	StartLocation sql.NullString `db:"start_location"`
	EndLocation   sql.NullString `db:"end_location"`
	Purpose       sql.NullString `db:"purpose"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type MileageRateRow struct {
	Year             int       `db:"year"`
	RateCentsPerMile int       `db:"rate_cents_per_mile"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

type DraftTransactionRow struct {
	ID        string         `db:"id"`
	ReceiptID string         `db:"receipt_id"`
	EntityID  string         `db:"entity_id"`
	Date      time.Time      `db:"date"`
	Memo      sql.NullString `db:"memo"`
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
}

type DraftEntryRow struct {
	ID                 string `db:"id"`
	DraftTransactionID string `db:"draft_transaction_id"`
	AccountID          string `db:"account_id"`
	DebitCents         int64  `db:"debit_cents"`
	CreditCents        int64  `db:"credit_cents"`
}

type VendorRuleRow struct {
	ID        string    `db:"id"`
	EntityID  string    `db:"entity_id"`
	MatchType string    `db:"match_type"`
	Pattern   string    `db:"pattern"`
	AccountID string    `db:"account_id"`
	CreatedAt time.Time `db:"created_at"`
}

type ReceiptOCRRow struct {
	ID         string         `db:"id"`
	ReceiptID  string         `db:"receipt_id"`
	Provider   string         `db:"provider"`
	Status     string         `db:"status"`
	RawText    sql.NullString `db:"raw_text"`
	DataJSON   []byte         `db:"data_json"`
	Error      sql.NullString `db:"error"`
	InputHash  sql.NullString `db:"input_hash"`
	RunVersion int            `db:"run_version"`
	CreatedAt  time.Time      `db:"created_at"`
}

type ReceiptSuggestionRow struct {
	ID               string          `db:"id"`
	ReceiptID        string          `db:"receipt_id"`
	Provider         string          `db:"provider"`
	Model            string          `db:"model"`
	Status           string          `db:"status"`
	PromptJSON       []byte          `db:"prompt_json"`
	RawJSON          []byte          `db:"raw_response"`
	ParsedJSON       []byte          `db:"parsed_json"`
	Confidence       sql.NullFloat64 `db:"confidence"`
	Error            sql.NullString  `db:"error"`
	InputHash        sql.NullString  `db:"input_hash"`
	RunVersion       int             `db:"run_version"`
	CreatedAt        time.Time       `db:"created_at"`
	PromptTokens     sql.NullInt64   `db:"prompt_tokens"`
	CompletionTokens sql.NullInt64   `db:"completion_tokens"`
	TotalTokens      sql.NullInt64   `db:"total_tokens"`
	CostCents        sql.NullInt64   `db:"cost_cents"`
}

type ReceiptJobRow struct {
	ID          string         `db:"id"`
	ReceiptID   string         `db:"receipt_id"`
	Stage       string         `db:"stage"`
	Status      string         `db:"status"`
	Attempts    int            `db:"attempts"`
	MaxAttempts int            `db:"max_attempts"`
	LockedUntil sql.NullTime   `db:"locked_until"`
	LockedBy    sql.NullString `db:"locked_by"`
	LastError   sql.NullString `db:"last_error"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

type ProcessingErrorRow struct {
	ID             string         `db:"id"`
	EntityID       string         `db:"entity_id"`
	ReceiptID      sql.NullString `db:"receipt_id"`
	MileageID      sql.NullString `db:"mileage_id"`
	Stage          string         `db:"stage"`
	Error          string         `db:"error"`
	CreatedAt      time.Time      `db:"created_at"`
	ResolvedAt     sql.NullTime   `db:"resolved_at"`
	ResolutionNote sql.NullString `db:"resolution_note"`
}

type TagRow struct {
	ID        string    `db:"id"`
	EntityID  string    `db:"entity_id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}
