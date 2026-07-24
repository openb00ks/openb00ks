package db

type Stores struct {
	DB                 *DB
	Users              *UserStore
	Tenants            *TenantStore
	TenantMemberships  *TenantMembershipStore
	TenantSettings     *TenantSettingsStore
	SystemSettings     *SystemSettingsStore
	Entities           *EntityStore
	EntityTaxSettings  *EntityTaxSettingsStore
	AccountRoleTags    *AccountRoleTagStore
	Accounts           *AccountStore
	AccountStatements  *AccountStatementStore
	Memberships        *MembershipStore
	Receipts           *ReceiptStore
	ImportRows         *ImportRowStore
	ReceiptJobs        *ReceiptJobStore
	ReceiptOCR         *ReceiptOCRStore
	ReceiptSuggestions *ReceiptSuggestionStore
	ReceiptMetadata    *ReceiptMetadataStore
	Tags               *TagStore
	ProcessingErrors   *ProcessingErrorStore
	Audit              *AuditStore
	Drafts             *DraftStore
	Transactions       *TransactionStore
	Reports            *ReportingStore
	Preferences        *PreferencesStore
	RefreshTokens      *RefreshTokenStore
	UserMFA            *UserMFAStore
	UsedTOTPSteps      *UsedTOTPStepStore
	Mileage            *MileageStore
	MileageRates       *MileageRateStore
	MileageMetadata    *MileageMetadataStore
	VendorRules        *VendorRuleStore
	Vendors            *VendorStore
	VendorAliases      *VendorAliasStore
	SearchSource       *SearchSource
	AdminStats         *AdminStatsStore
}

func NewStores(dbConn *DB) *Stores {
	if dbConn == nil {
		return nil
	}
	accountRoleTags := NewAccountRoleTagStore(dbConn)
	stores := &Stores{
		DB:                 dbConn,
		Users:              NewUserStore(dbConn),
		Tenants:            NewTenantStore(dbConn),
		TenantMemberships:  NewTenantMembershipStore(dbConn),
		TenantSettings:     NewTenantSettingsStore(dbConn),
		SystemSettings:     NewSystemSettingsStore(dbConn),
		Entities:           NewEntityStore(dbConn),
		EntityTaxSettings:  NewEntityTaxSettingsStore(dbConn),
		AccountRoleTags:    accountRoleTags,
		Accounts:           NewAccountStore(dbConn, accountRoleTags),
		AccountStatements:  NewAccountStatementStore(dbConn),
		Memberships:        NewMembershipStore(dbConn),
		Receipts:           NewReceiptStore(dbConn),
		ImportRows:         NewImportRowStore(dbConn),
		ReceiptJobs:        NewReceiptJobStore(dbConn),
		ReceiptOCR:         NewReceiptOCRStore(dbConn),
		ReceiptSuggestions: NewReceiptSuggestionStore(dbConn),
		ReceiptMetadata:    NewReceiptMetadataStore(dbConn),
		Tags:               NewTagStore(dbConn),
		ProcessingErrors:   NewProcessingErrorStore(dbConn),
		Audit:              NewAuditStore(dbConn),
		Drafts:             NewDraftStore(dbConn),
		Transactions:       NewTransactionStore(dbConn),
		Reports:            NewReportingStore(dbConn),
		Preferences:        NewPreferencesStore(dbConn),
		RefreshTokens:      NewRefreshTokenStore(dbConn),
		UserMFA:            NewUserMFAStore(dbConn),
		UsedTOTPSteps:      NewUsedTOTPStepStore(dbConn),
		Mileage:            NewMileageStore(dbConn),
		MileageRates:       NewMileageRateStore(dbConn),
		MileageMetadata:    NewMileageMetadataStore(dbConn),
		VendorRules:        NewVendorRuleStore(dbConn),
		Vendors:            NewVendorStore(dbConn),
		VendorAliases:      NewVendorAliasStore(dbConn),
	}
	stores.SearchSource = NewSearchSource(dbConn, stores.Accounts)
	stores.AdminStats = NewAdminStatsStore(dbConn)
	return stores
}
