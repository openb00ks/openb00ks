package httpapi

import (
	"time"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/queue"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
)

type HandlerContext struct {
	ready              ReadyChecker
	tokens             *auth.TokenService
	authn              auth.Authenticator
	jwtTTL             time.Duration
	refreshTTL         time.Duration
	corsAllowedOrigins []string
	users              UserStore
	admin              AdminChecker
	tenants            TenantStore
	tenantMembers      TenantMembershipStore
	entities           EntityStore
	entityTaxSettings  EntityTaxSettingsStore
	accounts           AccountStore
	accountStatements  AccountStatementStore
	memberships        MembershipStore
	systemSettings     SystemSettingsStore
	userMFA            UserMFAStore
	usedTOTPSteps      UsedTOTPStepStore
	receiptCfg         *ReceiptHandler
	receiptStore       ReceiptStore
	importRows         *db.ImportRowStore
	receiptJobs        *db.ReceiptJobStore
	receiptOCR         *db.ReceiptOCRStore
	suggestions        *db.ReceiptSuggestionStore
	receiptMeta        *db.ReceiptMetadataStore
	tags               *db.TagStore
	errors             *db.ProcessingErrorStore
	audit              *db.AuditStore
	drafts             *db.DraftStore
	reports            *db.ReportingStore
	transactions       TransactionStore
	preferences        *db.PreferencesStore
	refreshTokens      RefreshTokenStore
	aiPricing          suggest.Pricing
	mileage            *db.MileageStore
	mileageRates       *db.MileageRateStore
	mileageMeta        *db.MileageMetadataStore
	vendorRules        *db.VendorRuleStore
	vendors            *db.VendorStore
	vendorAliases      *db.VendorAliasStore
	adminStats         *db.AdminStatsStore
	search             searchpkg.Provider
	searchSource       searchpkg.TransactionSource
	objects            storage.ReceiptStore
	queue              queue.Queue
	systemInfo         SystemInfo
	metrics            *businessMetrics
}

type SetupStatus struct {
	Required bool `json:"required"`
}

type SystemInfo struct {
	AIProvider                string
	AIModel                   string
	ReceiptStorage            string
	ReceiptLocalDir           string
	ReceiptMaxBytes           int64
	PublicRegistrationEnabled bool
}

func NewHandlerContext(ready ReadyChecker, tokens *auth.TokenService, jwtTTL time.Duration, refreshTTL time.Duration, corsAllowedOrigins []string, pricing suggest.Pricing, objects storage.ReceiptStore, receiptCfg *ReceiptHandler, systemInfo SystemInfo) *HandlerContext {
	return &HandlerContext{
		ready:              ready,
		tokens:             tokens,
		jwtTTL:             jwtTTL,
		refreshTTL:         refreshTTL,
		corsAllowedOrigins: corsAllowedOrigins,
		aiPricing:          pricing,
		objects:            objects,
		receiptCfg:         receiptCfg,
		systemInfo:         systemInfo,
		metrics:            newBusinessMetrics(),
	}
}

func (hc *HandlerContext) SetSearchProvider(provider searchpkg.Provider) {
	hc.search = provider
}

func (hc *HandlerContext) SetStores(stores *db.Stores, q queue.Queue) {
	if stores == nil {
		hc.users = nil
		hc.admin = nil
		hc.tenants = nil
		hc.tenantMembers = nil
		hc.entities = nil
		hc.entityTaxSettings = nil
		hc.accounts = nil
		hc.accountStatements = nil
		hc.memberships = nil
		hc.systemSettings = nil
		hc.userMFA = nil
		hc.authn = nil
		hc.receiptStore = nil
		hc.importRows = nil
		hc.receiptJobs = nil
		hc.receiptOCR = nil
		hc.suggestions = nil
		hc.receiptMeta = nil
		hc.tags = nil
		hc.errors = nil
		hc.audit = nil
		hc.drafts = nil
		hc.reports = nil
		hc.transactions = nil
		hc.preferences = nil
		hc.refreshTokens = nil
		hc.mileage = nil
		hc.mileageRates = nil
		hc.mileageMeta = nil
		hc.vendorRules = nil
		hc.vendors = nil
		hc.vendorAliases = nil
		hc.adminStats = nil
		hc.searchSource = nil
		hc.queue = nil
		return
	}
	hc.users = stores.Users
	hc.admin = stores.Users
	hc.tenants = stores.Tenants
	hc.tenantMembers = stores.TenantMemberships
	hc.entities = stores.Entities
	hc.entityTaxSettings = stores.EntityTaxSettings
	hc.accounts = stores.Accounts
	hc.accountStatements = stores.AccountStatements
	hc.memberships = stores.Memberships
	hc.systemSettings = stores.SystemSettings
	hc.userMFA = stores.UserMFA
	hc.usedTOTPSteps = stores.UsedTOTPSteps
	hc.authn = db.NewAuthenticator(stores.Users)
	hc.receiptStore = stores.Receipts
	hc.importRows = stores.ImportRows
	hc.receiptJobs = stores.ReceiptJobs
	hc.receiptOCR = stores.ReceiptOCR
	hc.suggestions = stores.ReceiptSuggestions
	hc.receiptMeta = stores.ReceiptMetadata
	hc.tags = stores.Tags
	hc.errors = stores.ProcessingErrors
	hc.audit = stores.Audit
	hc.drafts = stores.Drafts
	hc.reports = stores.Reports
	hc.transactions = stores.Transactions
	hc.preferences = stores.Preferences
	hc.refreshTokens = stores.RefreshTokens
	hc.mileage = stores.Mileage
	hc.mileageRates = stores.MileageRates
	hc.mileageMeta = stores.MileageMetadata
	hc.vendorRules = stores.VendorRules
	hc.vendors = stores.Vendors
	hc.vendorAliases = stores.VendorAliases
	hc.adminStats = stores.AdminStats
	hc.searchSource = stores.SearchSource
	hc.queue = q
}
