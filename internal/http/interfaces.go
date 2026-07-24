package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type UserStore interface {
	Create(ctx context.Context, email, passwordHash string, isAdmin bool) (models.User, error)
	GetByID(ctx context.Context, id string) (models.User, error)
	GetByEmail(ctx context.Context, email string) (models.User, error)
	List(ctx context.Context, limit int) ([]models.User, error)
	Count(ctx context.Context) (int, error)
	SetDefaultTenant(ctx context.Context, userID, tenantID string) error
}

type AdminChecker interface {
	IsAdmin(ctx context.Context, userID string) (bool, error)
}

type EntityStore interface {
	ListForUser(ctx context.Context, tenantID, userID string, limit int) ([]models.Entity, error)
	CreateWithOwner(ctx context.Context, tenantID, userID, name, suggestionContext string) (models.Entity, error)
	Update(ctx context.Context, tenantID, entityID string, name *string, suggestionContext *string, fiscalYearStartMonth, fiscalYearStartDay *int) (models.Entity, error)
	Delete(ctx context.Context, tenantID, entityID string) error
	GetRole(ctx context.Context, tenantID, userID, entityID string) (models.Role, error)
}

type EntityTaxSettingsStore interface {
	Get(ctx context.Context, entityID string, taxYear int) (db.EntityTaxSettings, error)
	Upsert(ctx context.Context, entityID string, taxYear int, homeOfficeSqFt, homeTotalSqFt, cellPhoneBusinessUsePercent, homeInternetBusinessUsePercent sql.NullInt64) (db.EntityTaxSettings, error)
}

type TenantStore interface {
	Create(ctx context.Context, name string) (models.Tenant, error)
	GetByID(ctx context.Context, id string) (models.Tenant, error)
	Count(ctx context.Context) (int, error)
}

type TenantMembershipStore interface {
	ListForUser(ctx context.Context, userID string, limit int) ([]models.TenantMembership, error)
	Create(ctx context.Context, tenantID, userID string, role models.Role) (models.TenantMembership, error)
	GetRole(ctx context.Context, tenantID, userID string) (models.Role, error)
}

type SystemSettingsStore interface {
	Get(ctx context.Context) (db.SystemSettings, error)
	SetSetupComplete(ctx context.Context, completedAt time.Time) (db.SystemSettings, error)
	UpsertSettings(ctx context.Context, settingsJSON json.RawMessage) (db.SystemSettings, error)
}

type UserMFAStore interface {
	GetByUserID(ctx context.Context, userID string) (db.UserMFA, error)
	UpsertEnrollment(ctx context.Context, userID, secret string, recoveryCodeHashes json.RawMessage) (db.UserMFA, error)
	Enable(ctx context.Context, userID string) (db.UserMFA, error)
	Disable(ctx context.Context, userID string) (db.UserMFA, error)
	SetRecoveryCodeHashes(ctx context.Context, userID string, recoveryCodeHashes json.RawMessage) (db.UserMFA, error)
}

type UsedTOTPStepStore interface {
	MarkUsed(ctx context.Context, userID string, step int64, now time.Time) error
}

type RefreshTokenStore interface {
	Create(ctx context.Context, userID, tenantID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error)
	GetByHash(ctx context.Context, tokenHash string) (models.RefreshToken, error)
	Revoke(ctx context.Context, id string, usedAt time.Time) error
	RevokeIfActive(ctx context.Context, id string, revokedAt time.Time) (bool, error)
	RevokeAllForUser(ctx context.Context, userID string, revokedAt time.Time) error
}

type AccountStore interface {
	ListForEntity(ctx context.Context, entityID string, limit int) ([]models.Account, error)
	GetByID(ctx context.Context, accountID string) (models.Account, error)
	Create(ctx context.Context, entityID, name, typ, code string, roleTags ...string) (models.Account, error)
	Update(ctx context.Context, accountID, name, typ, code string, roleTags ...string) (models.Account, error)
	Delete(ctx context.Context, accountID string) error
	GetEntityID(ctx context.Context, accountID string) (string, error)
	FindDefaultCashAccount(ctx context.Context, entityID string) (models.Account, error)
}

type AccountStatementStore interface {
	List(ctx context.Context, entityID, accountID string, start, end *time.Time, limit int) ([]models.AccountStatement, error)
	GetByID(ctx context.Context, id string) (models.AccountStatement, error)
	GetBySourceReceiptID(ctx context.Context, receiptID string) (models.AccountStatement, error)
	GetEntityID(ctx context.Context, id string) (string, error)
	Create(ctx context.Context, statement models.AccountStatement) (models.AccountStatement, error)
	Update(ctx context.Context, id string, patch db.AccountStatementPatch) (models.AccountStatement, error)
	Reconcile(ctx context.Context, id string) (models.AccountStatement, error)
}

type MembershipStore interface {
	List(ctx context.Context, entityID string, limit int) ([]models.EntityUser, error)
	Create(ctx context.Context, entityID, userID string, role models.Role) (models.EntityUser, error)
	UpdateRole(ctx context.Context, entityUserID string, role models.Role) (models.EntityUser, error)
	Delete(ctx context.Context, entityUserID string) error
}

type ReceiptStore interface {
	Create(ctx context.Context, entityID, storageKey, contentType, status, kind, originalName string, sizeBytes int64, totalCents int64) (models.Receipt, error)
	GetByID(ctx context.Context, id string) (models.Receipt, error)
	GetEntityID(ctx context.Context, id string) (string, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateResolvedVendorID(ctx context.Context, id, vendorID string) error
	List(ctx context.Context, entityID string, status string, limit int) ([]models.Receipt, error)
	ListByKind(ctx context.Context, entityID, kind, status string, limit int) ([]models.Receipt, error)
}

type TransactionStore interface {
	Create(ctx context.Context, entityID string, date time.Time, memo string, receiptID string, entries []models.DraftEntry) (models.Transaction, []models.Entry, error)
	CreateFromDraft(ctx context.Context, draft models.DraftTransaction, receiptID string) (models.Transaction, []models.Entry, error)
	List(ctx context.Context, entityID string, start, end *time.Time, limit int) ([]db.TransactionWithEntries, error)
}
