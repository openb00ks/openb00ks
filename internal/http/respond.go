package httpapi

import "github.com/gin-gonic/gin"

// API error codes. Every error response uses one of these constants so a
// typo is a compile error and the set is discoverable in one place.
const (
	CodeAccountInUse               = "ACCOUNT_IN_USE"
	CodeBadRequest                 = "BAD_REQUEST"
	CodeDbUnavailable              = "DB_UNAVAILABLE"
	CodeDefaultCashAccountRequired = "DEFAULT_CASH_ACCOUNT_REQUIRED"
	CodeDuplicateVendor            = "DUPLICATE_VENDOR"
	CodeEmailAlreadyExists         = "EMAIL_ALREADY_EXISTS"
	CodeFileTooLarge               = "FILE_TOO_LARGE"
	CodeForbidden                  = "FORBIDDEN"
	CodeImportRowAccountRequired   = "IMPORT_ROW_ACCOUNT_REQUIRED"
	CodeImportRowAlreadyPosted     = "IMPORT_ROW_ALREADY_POSTED"
	CodeInternalError              = "INTERNAL_ERROR"
	CodeInvalidAccount             = "INVALID_ACCOUNT"
	CodeInvalidCredentials         = "INVALID_CREDENTIALS"
	CodeInvalidDate                = "INVALID_DATE"
	CodeInvalidDraft               = "INVALID_DRAFT"
	CodeInvalidEntry               = "INVALID_ENTRY"
	CodeInvalidFileType            = "INVALID_FILE_TYPE"
	CodeInvalidMatchType           = "INVALID_MATCH_TYPE"
	CodeInvalidMfaChallenge        = "INVALID_MFA_CHALLENGE"
	CodeInvalidMfaCode             = "INVALID_MFA_CODE"
	CodeInvalidPeriod              = "INVALID_PERIOD"
	CodeInvalidRefreshToken        = "INVALID_REFRESH_TOKEN"
	CodeInvalidRole                = "INVALID_ROLE"
	CodeInvalidRoleTags            = "INVALID_ROLE_TAGS"
	CodeInvalidRowIndex            = "INVALID_ROW_INDEX"
	CodeInvalidSourceImport        = "INVALID_SOURCE_IMPORT"
	CodeInvalidStatus              = "INVALID_STATUS"
	CodeInvalidTaxYear             = "INVALID_TAX_YEAR"
	CodeInvalidTemplate            = "INVALID_TEMPLATE"
	CodeInvalidTheme               = "INVALID_THEME"
	CodeInvalidTransaction         = "INVALID_TRANSACTION"
	CodeInvalidVendor              = "INVALID_VENDOR"
	CodeInvalidYear                = "INVALID_YEAR"
	CodeMfaChallengeExpired        = "MFA_CHALLENGE_EXPIRED"
	CodeMfaCodeAlreadyUsed         = "MFA_CODE_ALREADY_USED"
	CodeMfaSetupRequired           = "MFA_SETUP_REQUIRED"
	CodeMissingFields              = "MISSING_FIELDS"
	CodeNoSuggestion               = "NO_SUGGESTION"
	CodeNotFound                   = "NOT_FOUND"
	CodeNotImplemented             = "NOT_IMPLEMENTED"
	CodePasswordTooShort           = "PASSWORD_TOO_SHORT"
	CodeReceiptAlreadyAttached     = "RECEIPT_ALREADY_ATTACHED"
	CodeRegistrationDisabled       = "REGISTRATION_DISABLED"
	CodeSearchNotConfigured        = "SEARCH_NOT_CONFIGURED"
	CodeSearchReindexFailed        = "SEARCH_REINDEX_FAILED"
	CodeSearchUnavailable          = "SEARCH_UNAVAILABLE"
	CodeSetupAlreadyComplete       = "SETUP_ALREADY_COMPLETE"
	CodeSetupRequired              = "SETUP_REQUIRED"
	CodeTenantNotFound             = "TENANT_NOT_FOUND"
	CodeUnauthorized               = "UNAUTHORIZED"
	CodeUserNotFound               = "USER_NOT_FOUND"
)

// respondError writes the standard { "error": <code> } envelope with the given
// HTTP status.
func respondError(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": code})
}
