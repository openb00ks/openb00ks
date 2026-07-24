package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/templates"
	"github.com/spectrum-labs-tech/go-toolkit/pkg/ginmiddleware"
)

type ReadyChecker interface {
	Ready(ctx context.Context) error
}

type Server struct {
	engine *gin.Engine
	hc     *HandlerContext
}

func NewServer(hc *HandlerContext) *Server {
	s := &Server{
		engine: gin.New(),
		hc:     hc,
	}
	s.engine.Use(RequestTracing("openb00ks-api"))
	s.engine.Use(RequestMetrics("openb00ks-api"))
	s.engine.Use(RequestLogger())
	s.engine.Use(gin.Recovery())
	s.engine.Use(ginmiddleware.SecureHeaders(ginmiddleware.WithoutHSTS()))
	s.engine.Use(ginmiddleware.RequestSizeLimit(50 << 20)) // 50 MB global cap
	s.engine.Use(hc.cors())
	s.engine.Use(hc.requireDB())
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.engine
}

func (s *Server) routes() {
	hc := s.hc
	s.engine.GET("/healthz", hc.handleHealth)
	s.engine.GET("/readyz", hc.handleReady)
	s.engine.GET("/setup/status", hc.handleSetupStatus)
	// Account-set templates for the entity-create pickers — public so the first-run setup page can list
	// them before anyone has authenticated.
	s.engine.GET("/entity-templates", hc.handleListEntityTemplates)

	// Rate-limit auth and setup endpoints: burst of 5 then 0.5 req/s per IP.
	// Disabled in test mode so integration tests don't exhaust their own burst.
	authRL := ginmiddleware.IPRateLimit(ginmiddleware.NewRateLimiter(0.5, 5))
	if gin.Mode() == gin.TestMode {
		authRL = func(c *gin.Context) { c.Next() }
	}
	s.engine.POST("/setup", authRL, hc.handleSetup)
	s.engine.POST("/auth/register", authRL, hc.handleRegister)
	s.engine.POST("/auth/login", authRL, hc.handleLogin)
	s.engine.POST("/auth/login/mfa", authRL, hc.handleLoginMFA)
	s.engine.POST("/auth/refresh", authRL, hc.handleRefresh)
	s.engine.POST("/auth/logout", hc.handleLogout)

	auth := s.engine.Group("/")
	auth.Use(AuthRequired(hc.tokens), hc.requireTenantMembership())
	auth.POST("/auth/logout-all", hc.handleLogoutAll)
	auth.GET("/tenants", hc.handleListTenants)
	auth.POST("/tenants/:id/switch", hc.requireTenantAccess("id"), hc.handleSwitchTenant)
	auth.GET("/me", hc.handleMe)
	auth.GET("/me/mfa", hc.handleMFAStatus)
	auth.POST("/me/mfa/setup", hc.handleMFASetup)
	auth.POST("/me/mfa/confirm", hc.handleMFAConfirm)
	auth.DELETE("/me/mfa", hc.handleMFADisable)

	admin := auth.Group("/")
	admin.Use(AdminRequired(hc.admin))

	auth.GET("/entities", hc.handleListEntities)
	auth.POST("/entities", hc.handleCreateEntity)
	auth.PATCH("/entities/:id", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleUpdateEntity)
	auth.GET("/entities/:id/tax-settings", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleGetEntityTaxSettings)
	auth.PATCH("/entities/:id/tax-settings", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleUpdateEntityTaxSettings)
	auth.DELETE("/entities/:id", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleDeleteEntity)

	auth.GET("/entities/:id/members", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleListMembers)
	auth.POST("/entities/:id/members", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleCreateMember)
	auth.PATCH("/entities/:id/members/:member_id", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleUpdateMember)
	auth.DELETE("/entities/:id/members/:member_id", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleDeleteMember)

	auth.GET("/entities/:id/accounts", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleListAccounts)
	auth.GET("/entities/:id/account-balances", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleAccountBalances)
	auth.GET("/accounts/:id/transactions", hc.requireEntityRole(adminRoles(), hc.entityIDFromAccountParam("id")), hc.handleAccountTransactions)
	auth.POST("/entities/:id/accounts", hc.requireEntityRole(adminRoles(), entityIDFromParam("id")), hc.handleCreateAccount)
	auth.PATCH("/accounts/:id", hc.requireEntityRole(adminRoles(), hc.entityIDFromAccountParam("id")), hc.handleUpdateAccount)
	auth.DELETE("/accounts/:id", hc.requireEntityRole(adminRoles(), hc.entityIDFromAccountParam("id")), hc.handleDeleteAccount)
	auth.GET("/account-statements", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleAccountStatementsList)
	auth.POST("/account-statements", hc.requireEntityRole(adminRoles(), entityIDFromJSON("entity_id")), hc.handleAccountStatementCreate)
	auth.PATCH("/account-statements/:id", hc.requireEntityRole(adminRoles(), hc.entityIDFromAccountStatementParam("id")), hc.handleAccountStatementUpdate)
	auth.POST("/account-statements/:id/reconcile", hc.requireEntityRole(adminRoles(), hc.entityIDFromAccountStatementParam("id")), hc.handleAccountStatementReconcile)

	auth.POST("/receipts", hc.requireEntityRole(memberRoles(), entityIDFromForm("entity_id")), hc.handleReceiptUpload)
	auth.GET("/receipts", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleReceiptList)
	auth.GET("/receipts/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptGet)
	auth.GET("/receipts/:id/status", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptStatus)
	auth.PATCH("/receipts/:id/tags", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptTagsUpdate)
	auth.POST("/receipts/suggestions/batch", hc.requireReceiptsRole(memberRoles(), receiptIDsFromBody("receipt_ids")), hc.handleReceiptSuggestionsBatch)
	auth.POST("/receipts/:id/requeue", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptRequeue)
	auth.GET("/receipts/:id/ocr", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptOCR)
	auth.POST("/receipts/:id/ocr/rerun", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptOCRRerun)
	auth.GET("/receipts/:id/suggestion", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptSuggestion)
	auth.POST("/receipts/:id/suggestion/rerun", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptSuggestionRerun)
	auth.POST("/receipts/:id/draft/rerun", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptDraftRerun)
	auth.GET("/receipts/:id/draft", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleDraftGet)
	auth.POST("/receipts/:id/post", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleDraftPost)
	auth.PATCH("/receipts/:id/draft", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleDraftUpdate)
	auth.PATCH("/receipts/:id/vendor", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptSetVendor)

	auth.POST("/imports", hc.requireEntityRole(memberRoles(), entityIDFromForm("entity_id")), hc.handleImportUpload)
	auth.GET("/imports", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleImportList)
	auth.GET("/imports/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportGet)
	auth.GET("/imports/:id/rows", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportRowsList)
	auth.PATCH("/imports/:id/rows/:row_index", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportRowUpdate)
	auth.POST("/imports/:id/rows/post-mapped", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportRowsPostMapped)
	auth.POST("/imports/:id/rows/:row_index/post", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportRowPost)
	auth.GET("/imports/:id/ocr", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptOCR)
	auth.POST("/imports/:id/ocr/rerun", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptOCRRerun)
	auth.GET("/imports/:id/suggestion", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportSuggestion)
	auth.POST("/imports/:id/suggestion/rerun", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleImportSuggestionRerun)
	auth.POST("/imports/:id/requeue", hc.requireEntityRole(adminRoles(), hc.entityIDFromReceiptParam("id")), hc.handleReceiptRequeue)
	auth.GET("/reports/general-ledger", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportGeneralLedger)
	auth.GET("/reports/profit-loss", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportProfitLoss)
	auth.GET("/reports/balance-sheet", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportBalanceSheet)
	auth.GET("/reports/trial-balance", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportTrialBalance)
	auth.GET("/reports/vendor-payments", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportVendorPayments)
	auth.GET("/reports/tax-readiness", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleReportTaxReadiness)
	auth.GET("/exports/transactions.csv", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleExportTransactionsCSV)
	auth.GET("/exports/tax-pack.zip", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleExportTaxPack)
	auth.GET("/me/preferences", hc.handlePreferencesGet)
	auth.PATCH("/me/preferences", hc.requireOptionalEntityRole(memberRoles(), "default_entity_id"), hc.handlePreferencesUpdate)
	auth.GET("/mileage", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleMileageList)
	auth.POST("/mileage", hc.requireEntityRole(memberRoles(), entityIDFromJSON("entity_id")), hc.handleMileageCreate)
	auth.PATCH("/mileage/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromMileageParam("id")), hc.handleMileageUpdate)
	auth.DELETE("/mileage/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromMileageParam("id")), hc.handleMileageDelete)
	auth.GET("/exports/mileage.csv", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleMileageExport)
	auth.GET("/reports/mileage", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleMileageSummary)
	auth.GET("/mileage-rates", hc.handleMileageRatesList)
	auth.GET("/mileage-rates/:year", hc.handleMileageRatesGet)
	admin.PUT("/mileage-rates/:year", hc.handleMileageRatesUpsert)

	auth.POST("/transactions", hc.requireEntityRole(memberRoles(), entityIDFromJSON("entity_id")), hc.handleTransactionCreate)
	auth.GET("/transactions", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleTransactionList)
	auth.GET("/search", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleSearch)
	auth.POST("/search/reindex", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleSearchReindex)
	auth.GET("/search/transactions", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleTransactionSearch)
	auth.POST("/search/transactions/reindex", hc.requireEntityRole(adminRoles(), entityIDFromQuery("entity_id")), hc.handleTransactionSearchReindex)

	auth.GET("/vendor-rules", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleVendorRulesList)
	auth.POST("/vendor-rules", hc.requireEntityRole(memberRoles(), entityIDFromJSON("entity_id")), hc.handleVendorRulesCreate)
	auth.PATCH("/vendor-rules/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromVendorRuleParam("id")), hc.handleVendorRulesUpdate)
	auth.DELETE("/vendor-rules/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromVendorRuleParam("id")), hc.handleVendorRulesDelete)

	auth.GET("/vendors", hc.requireEntityRole(memberRoles(), entityIDFromQuery("entity_id")), hc.handleVendorsList)
	auth.POST("/vendors", hc.requireEntityRole(memberRoles(), entityIDFromJSON("entity_id")), hc.handleVendorsCreate)
	auth.GET("/vendors/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromVendorParam("id")), hc.handleVendorsGet)
	auth.PATCH("/vendors/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromVendorParam("id")), hc.handleVendorsUpdate)
	auth.DELETE("/vendors/:id", hc.requireEntityRole(memberRoles(), hc.entityIDFromVendorParam("id")), hc.handleVendorsDelete)

	auth.POST("/suggest", hc.requireEntityRole(memberRoles(), hc.entityIDFromReceiptJSON("receipt_id")), hc.handleSuggest)

	admin.GET("/settings/system", hc.handleSystemSettingsGet)
	admin.PATCH("/settings/system", hc.handleSystemSettingsUpdate)

	admin.GET("/migrations", hc.handleMigrationStatus)
	admin.POST("/migrations/up", hc.handleMigrationUp)
	admin.POST("/users", hc.handleCreateUser)
	admin.GET("/users", hc.handleListUsers)
	admin.POST("/users/:id/mfa/reset", hc.handleResetUserMFA)

	admin.GET("/admin/stats", hc.handleAdminStats)
	admin.GET("/admin/queue/jobs", hc.handleAdminJobsList)
	admin.POST("/admin/queue/jobs/:id/requeue", hc.handleAdminJobRequeue)
	admin.GET("/admin/processing-errors", hc.handleAdminErrorsList)
	admin.POST("/admin/processing-errors/:id/resolve", hc.handleAdminErrorResolve)
}

func (hc *HandlerContext) handleHealth(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (hc *HandlerContext) handleReady(c *gin.Context) {
	if hc.ready == nil {
		c.String(http.StatusOK, "ok")
		return
	}
	if err := hc.ready.Ready(c.Request.Context()); err != nil {
		c.String(http.StatusServiceUnavailable, "not ready")
		return
	}
	c.String(http.StatusOK, "ok")
}

func (hc *HandlerContext) requireDB() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.URL.Path {
		case "/healthz", "/readyz":
			c.Next()
			return
		}
		if hc.ready == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": CodeDbUnavailable})
			return
		}
		if err := hc.ready.Ready(c.Request.Context()); err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": CodeDbUnavailable})
			return
		}
		c.Next()
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
}

type registerRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantName string `json:"tenant_name"`
}

type loginResponse struct {
	Token            string `json:"token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	TenantID         string `json:"tenant_id"`
	MFARequired      bool   `json:"mfa_required,omitempty"`
	ChallengeToken   string `json:"challenge_token,omitempty"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type loginMFARequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type createUserRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	IsAdmin    bool   `json:"is_admin"`
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
}

type userResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	IsAdmin       bool   `json:"is_admin"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	MFAConfigured bool   `json:"mfa_configured"`
	CreatedAt     string `json:"created_at"`
}

type entityResponse struct {
	ID                   string `json:"id"`
	TenantID             string `json:"tenant_id"`
	Name                 string `json:"name"`
	SuggestionContext    string `json:"suggestion_context"`
	FiscalYearStartMonth int    `json:"fiscal_year_start_month"`
	FiscalYearStartDay   int    `json:"fiscal_year_start_day"`
	CreatedAt            string `json:"created_at"`
}

type tenantResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type createEntityRequest struct {
	Name              string `json:"name"`
	Template          string `json:"template"`
	SuggestionContext string `json:"suggestion_context"`
}

type updateEntityRequest struct {
	Name                 *string `json:"name"`
	SuggestionContext    *string `json:"suggestion_context"`
	FiscalYearStartMonth *int    `json:"fiscal_year_start_month"`
	FiscalYearStartDay   *int    `json:"fiscal_year_start_day"`
}

type accountResponse struct {
	ID        string   `json:"id"`
	EntityID  string   `json:"entity_id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Code      string   `json:"code,omitempty"`
	RoleTags  []string `json:"role_tags"`
	CreatedAt string   `json:"created_at"`
}

type createAccountRequest struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Code     string   `json:"code"`
	RoleTags []string `json:"role_tags"`
}

type updateAccountRequest struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Code     string   `json:"code"`
	RoleTags []string `json:"role_tags"`
}

type memberResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	EntityID  string `json:"entity_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type createMemberRequest struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type updateMemberRequest struct {
	Role string `json:"role"`
}

var errEmailAlreadyExists = errors.New("email already exists")

func (hc *HandlerContext) handleRegister(c *gin.Context) {
	if hc.tokens == nil || hc.refreshTokens == nil || hc.users == nil || hc.tenants == nil || hc.tenantMembers == nil || hc.systemSettings == nil {
		hc.notImplemented(c)
		return
	}
	if !hc.systemInfo.PublicRegistrationEnabled {
		respondError(c, http.StatusForbidden, CodeRegistrationDisabled)
		return
	}
	required, err := hc.setupRequired(c)
	if err != nil {
		if errors.Is(err, db.ErrUnavailable) {
			hc.notImplemented(c)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if required {
		respondError(c, http.StatusConflict, CodeSetupRequired)
		return
	}

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	password := req.Password
	tenantName := strings.TrimSpace(req.TenantName)
	if email == "" || password == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if len(password) < auth.MinPasswordLen {
		respondError(c, http.StatusBadRequest, CodePasswordTooShort)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	userID, tenantID, err := hc.registerUser(c.Request.Context(), email, hash, tenantName)
	if err != nil {
		if errors.Is(err, errEmailAlreadyExists) {
			respondError(c, http.StatusConflict, CodeEmailAlreadyExists)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	resp, err := hc.issueSession(c.Request.Context(), userID, tenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (hc *HandlerContext) handleLogin(c *gin.Context) {
	if hc.authn == nil || hc.tokens == nil || hc.refreshTokens == nil || hc.users == nil || hc.tenantMembers == nil {
		hc.notImplemented(c)
		return
	}
	if required, err := hc.setupRequired(c); err == nil && required {
		respondError(c, http.StatusConflict, CodeSetupRequired)
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	userID, err := hc.authn.Authenticate(c.Request.Context(), email, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			respondError(c, http.StatusUnauthorized, CodeInvalidCredentials)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	tenantID, err := hc.resolveTenantForLogin(c.Request.Context(), userID, req.TenantID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusForbidden, CodeTenantNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	if hc.userMFA != nil {
		mfaRecord, mfaErr := hc.userMFA.GetByUserID(c.Request.Context(), userID)
		if mfaErr != nil && !errors.Is(mfaErr, db.ErrNotFound) {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		requiresMFA, settingsErr := hc.systemRequiresMFA(c)
		if settingsErr != nil && !errors.Is(settingsErr, db.ErrUnavailable) {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		if (mfaRecord.Enabled && mfaRecord.Secret != "") || requiresMFA {
			if mfaRecord.Secret == "" || !mfaRecord.Enabled {
				respondError(c, http.StatusConflict, CodeMfaSetupRequired)
				return
			}
			challengeToken, err := hc.tokens.IssueChallenge(userID, tenantID, auth.MFATokenPurpose, auth.MFATokenTTL)
			if err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
			c.JSON(http.StatusOK, loginResponse{
				MFARequired:    true,
				ChallengeToken: challengeToken,
				TenantID:       tenantID,
			})
			return
		}
	}

	resp, err := hc.issueSession(c.Request.Context(), userID, tenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleRefresh(c *gin.Context) {
	if hc.tokens == nil || hc.refreshTokens == nil {
		hc.notImplemented(c)
		return
	}
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.RefreshToken == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	now := time.Now()
	hash := auth.HashRefreshToken(req.RefreshToken)
	stored, err := hc.refreshTokens.GetByHash(c.Request.Context(), hash)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusUnauthorized, CodeInvalidRefreshToken)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if stored.RevokedAt != nil || now.After(stored.ExpiresAt) {
		respondError(c, http.StatusUnauthorized, CodeInvalidRefreshToken)
		return
	}

	// Atomically revoke before issuing — prevents a concurrent request from
	// exchanging the same token twice.
	revoked, err := hc.refreshTokens.RevokeIfActive(c.Request.Context(), stored.ID, now)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if !revoked {
		respondError(c, http.StatusUnauthorized, CodeInvalidRefreshToken)
		return
	}

	resp, err := hc.issueSession(c.Request.Context(), stored.UserID, stored.TenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleLogout(c *gin.Context) {
	if hc.refreshTokens == nil {
		hc.notImplemented(c)
		return
	}
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.RefreshToken == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	now := time.Now()
	hash := auth.HashRefreshToken(req.RefreshToken)
	stored, err := hc.refreshTokens.GetByHash(c.Request.Context(), hash)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if stored.RevokedAt == nil {
		if err := hc.refreshTokens.Revoke(c.Request.Context(), stored.ID, now); err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleLogoutAll(c *gin.Context) {
	if hc.refreshTokens == nil {
		hc.notImplemented(c)
		return
	}
	userID := userIDFromContext(c)
	now := time.Now()
	if err := hc.refreshTokens.RevokeAllForUser(c.Request.Context(), userID, now); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

// handleMe returns the signed-in user's identity for the UI (header, profile, admin gating). It never
// exposes the password hash.
func (hc *HandlerContext) handleMe(c *gin.Context) {
	if hc.users == nil {
		hc.notImplemented(c)
		return
	}
	user, err := hc.users.GetByID(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                user.ID,
		"email":             user.Email,
		"is_admin":          user.IsAdmin,
		"default_tenant_id": user.DefaultTenantID,
		"created_at":        user.CreatedAt.Format(time.RFC3339),
	})
}

func (hc *HandlerContext) handleListTenants(c *gin.Context) {
	if hc.tenantMembers == nil {
		hc.notImplemented(c)
		return
	}
	userID := userIDFromContext(c)
	limit := queryLimit(c, 100, 1000)
	memberships, err := hc.tenantMembers.ListForUser(c.Request.Context(), userID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]tenantResponse, 0, len(memberships))
	for _, membership := range memberships {
		resp = append(resp, tenantResponse{
			ID:        membership.TenantID,
			Name:      membership.TenantName,
			Role:      string(membership.Role),
			CreatedAt: membership.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleSwitchTenant(c *gin.Context) {
	if hc.tokens == nil || hc.refreshTokens == nil || hc.tenantMembers == nil {
		hc.notImplemented(c)
		return
	}
	userID := userIDFromContext(c)
	tenantID, _ := TargetTenantID(c)

	resp, err := hc.issueSession(c.Request.Context(), userID, tenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleCreateUser(c *gin.Context) {
	if hc.users == nil {
		hc.notImplemented(c)
		return
	}
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if len(req.Password) < auth.MinPasswordLen {
		respondError(c, http.StatusBadRequest, CodePasswordTooShort)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	user, err := hc.users.Create(c.Request.Context(), req.Email, hash, req.IsAdmin)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID != "" {
		if hc.tenants == nil || hc.tenantMembers == nil {
			respondError(c, http.StatusNotImplemented, CodeNotImplemented)
			return
		}
		if _, err := hc.tenants.GetByID(c.Request.Context(), tenantID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				respondError(c, http.StatusBadRequest, CodeTenantNotFound)
				return
			}
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
	} else if hc.tenants != nil && hc.tenantMembers != nil {
		name := strings.TrimSpace(req.TenantName)
		if name == "" {
			name = "Default Tenant"
		}
		tenant, err := hc.tenants.Create(c.Request.Context(), name)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		tenantID = tenant.ID
	}

	if tenantID != "" && hc.tenantMembers != nil {
		role := models.RoleUser
		if req.IsAdmin {
			role = models.RoleAdmin
		}
		if _, err := hc.tenantMembers.Create(c.Request.Context(), tenantID, user.ID, role); err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		if err := hc.users.SetDefaultTenant(c.Request.Context(), user.ID, tenantID); err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
	}

	c.JSON(http.StatusCreated, userResponse{
		ID:        user.ID,
		Email:     user.Email,
		IsAdmin:   user.IsAdmin,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	})
}

func (hc *HandlerContext) handleListEntities(c *gin.Context) {
	if hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	userID := userIDFromContext(c)
	tenantID := tenantIDFromContext(c)
	limit := queryLimit(c, 100, 1000)
	entities, err := hc.entities.ListForUser(c.Request.Context(), tenantID, userID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]entityResponse, 0, len(entities))
	for _, entity := range entities {
		resp = append(resp, entityResponseFromModel(entity))
	}
	c.JSON(http.StatusOK, resp)
}

func entityResponseFromModel(entity models.Entity) entityResponse {
	return entityResponse{
		ID:                   entity.ID,
		TenantID:             entity.TenantID,
		Name:                 entity.Name,
		SuggestionContext:    entity.SuggestionContext,
		FiscalYearStartMonth: entity.FiscalYearStartMonth,
		FiscalYearStartDay:   entity.FiscalYearStartDay,
		CreatedAt:            entity.CreatedAt.Format(time.RFC3339),
	}
}

func validateFiscalYearStart(month, day *int) error {
	if month != nil && (*month < 1 || *month > 12) {
		return errors.New("INVALID_FISCAL_YEAR_START_MONTH")
	}
	if day != nil && (*day < 1 || *day > 31) {
		return errors.New("INVALID_FISCAL_YEAR_START_DAY")
	}
	if month == nil || day == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", fmt.Sprintf("2024-%02d-%02d", *month, *day)); err != nil {
		return errors.New("INVALID_FISCAL_YEAR_START")
	}
	return nil
}

func (hc *HandlerContext) handleCreateEntity(c *gin.Context) {
	if hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	userID := userIDFromContext(c)
	tenantID := tenantIDFromContext(c)
	var req createEntityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Name == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	defs, err := resolveTemplateDefs(req.Template)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidTemplate)
		return
	}
	entity, err := hc.entities.CreateWithOwner(c.Request.Context(), tenantID, userID, req.Name, req.SuggestionContext)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.seedEntityAccounts(c.Request.Context(), entity.ID, defs)
	c.JSON(http.StatusCreated, entityResponseFromModel(entity))
}

// handleListEntityTemplates returns the account-set templates for the entity-create pickers. Public
// (the first-run setup page is unauthenticated) and non-sensitive — just static starter charts.
func (hc *HandlerContext) handleListEntityTemplates(c *gin.Context) {
	list := templates.List()
	rows := make([]gin.H, 0, len(list))
	for _, tmpl := range list {
		rows = append(rows, gin.H{
			"key":           tmpl.Key,
			"name":          tmpl.Name,
			"account_count": len(tmpl.Accounts),
		})
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

func (hc *HandlerContext) handleUpdateEntity(c *gin.Context) {
	if hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	tenantID := tenantIDFromContext(c)
	entityID := c.Param("id")
	var req updateEntityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Name == nil && req.SuggestionContext == nil && req.FiscalYearStartMonth == nil && req.FiscalYearStartDay == nil {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if err := validateFiscalYearStart(req.FiscalYearStartMonth, req.FiscalYearStartDay); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entity, err := hc.entities.Update(c.Request.Context(), tenantID, entityID, req.Name, req.SuggestionContext, req.FiscalYearStartMonth, req.FiscalYearStartDay)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, entityResponseFromModel(entity))
}

func (hc *HandlerContext) handleDeleteEntity(c *gin.Context) {
	if hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	tenantID := tenantIDFromContext(c)
	entityID := c.Param("id")
	if err := hc.entities.Delete(c.Request.Context(), tenantID, entityID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleListUsers(c *gin.Context) {
	if hc.users == nil {
		hc.notImplemented(c)
		return
	}
	limit := queryLimit(c, 100, 1000)
	users, err := hc.users.List(c.Request.Context(), limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]userResponse, 0, len(users))
	for _, user := range users {
		var enabled bool
		var configured bool
		if hc.userMFA != nil {
			if record, err := hc.userMFA.GetByUserID(c.Request.Context(), user.ID); err == nil {
				enabled = record.Enabled
				configured = record.Secret != ""
			}
		}
		resp = append(resp, userResponse{
			ID:            user.ID,
			Email:         user.Email,
			IsAdmin:       user.IsAdmin,
			MFAEnabled:    enabled,
			MFAConfigured: configured,
			CreatedAt:     user.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleResetUserMFA(c *gin.Context) {
	if hc.userMFA == nil || hc.users == nil {
		hc.notImplemented(c)
		return
	}
	userID := c.Param("id")
	_, err := hc.users.GetByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	record, err := hc.userMFA.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusConflict, CodeMfaSetupRequired)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if record.Secret == "" {
		respondError(c, http.StatusConflict, CodeMfaSetupRequired)
		return
	}
	_, err = hc.userMFA.Disable(c.Request.Context(), userID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleListAccounts(c *gin.Context) {
	if hc.accounts == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	entityID := c.Param("id")
	limit := queryLimit(c, 200, 1000)
	accounts, err := hc.accounts.ListForEntity(c.Request.Context(), entityID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, accountResponseFromModel(account))
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleListMembers(c *gin.Context) {
	if hc.memberships == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	entityID := c.Param("id")
	limit := queryLimit(c, 100, 1000)
	members, err := hc.memberships.List(c.Request.Context(), entityID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]memberResponse, 0, len(members))
	for _, member := range members {
		resp = append(resp, memberResponse{
			ID:        member.ID,
			UserID:    member.UserID,
			EntityID:  member.EntityID,
			Role:      string(member.Role),
			CreatedAt: member.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleCreateMember(c *gin.Context) {
	if hc.memberships == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	tenantID := tenantIDFromContext(c)
	entityID := c.Param("id")
	var req createMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if (req.UserID == "" && req.Email == "") || req.Role == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	// Prefer adding by email (the UI's path); fall back to a raw user_id.
	if req.UserID == "" {
		if hc.users == nil {
			hc.notImplemented(c)
			return
		}
		user, err := hc.users.GetByEmail(c.Request.Context(), strings.TrimSpace(req.Email))
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				respondError(c, http.StatusNotFound, CodeUserNotFound)
				return
			}
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		req.UserID = user.ID
	}
	role := models.Role(req.Role)
	switch role {
	case models.RoleAdmin, models.RoleAccountant, models.RoleUser:
	default:
		respondError(c, http.StatusBadRequest, CodeInvalidRole)
		return
	}
	member, err := hc.memberships.Create(c.Request.Context(), entityID, req.UserID, role)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if hc.tenantMembers != nil {
		if _, err := hc.tenantMembers.GetRole(c.Request.Context(), tenantID, req.UserID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				tenantRole := models.RoleUser
				if member.Role == models.RoleAdmin {
					tenantRole = models.RoleAdmin
				}
				if _, err := hc.tenantMembers.Create(c.Request.Context(), tenantID, req.UserID, tenantRole); err != nil {
					respondError(c, http.StatusInternalServerError, CodeInternalError)
					return
				}
			} else {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
		}
	}
	c.JSON(http.StatusCreated, memberResponse{
		ID:        member.ID,
		UserID:    member.UserID,
		EntityID:  member.EntityID,
		Role:      string(member.Role),
		CreatedAt: member.CreatedAt.Format(time.RFC3339),
	})
}

func (hc *HandlerContext) handleUpdateMember(c *gin.Context) {
	if hc.memberships == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	var req updateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	role := models.Role(req.Role)
	switch role {
	case models.RoleAdmin, models.RoleAccountant, models.RoleUser:
	default:
		respondError(c, http.StatusBadRequest, CodeInvalidRole)
		return
	}
	memberID := c.Param("member_id")
	member, err := hc.memberships.UpdateRole(c.Request.Context(), memberID, role)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, memberResponse{
		ID:        member.ID,
		UserID:    member.UserID,
		EntityID:  member.EntityID,
		Role:      string(member.Role),
		CreatedAt: member.CreatedAt.Format(time.RFC3339),
	})
}

func (hc *HandlerContext) handleDeleteMember(c *gin.Context) {
	if hc.memberships == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	memberID := c.Param("member_id")
	if err := hc.memberships.Delete(c.Request.Context(), memberID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleCreateAccount(c *gin.Context) {
	if hc.accounts == nil || hc.entities == nil {
		hc.notImplemented(c)
		return
	}
	entityID := c.Param("id")
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Name == "" || req.Type == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	roleTags, ok := normalizeAccountRoleTags(req.RoleTags)
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRoleTags)
		return
	}
	account, err := hc.accounts.Create(c.Request.Context(), entityID, req.Name, req.Type, req.Code, roleTags...)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexAccount(c, account)
	c.JSON(http.StatusCreated, accountResponseFromModel(account))
}

func (hc *HandlerContext) handleUpdateAccount(c *gin.Context) {
	if hc.accounts == nil {
		hc.notImplemented(c)
		return
	}
	var req updateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Name == "" || req.Type == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	accountID := c.Param("id")
	roleTags, ok := normalizeAccountRoleTags(req.RoleTags)
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRoleTags)
		return
	}
	account, err := hc.accounts.Update(c.Request.Context(), accountID, req.Name, req.Type, req.Code, roleTags...)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexAccount(c, account)
	c.JSON(http.StatusOK, accountResponseFromModel(account))
}

func (hc *HandlerContext) handleDeleteAccount(c *gin.Context) {
	if hc.accounts == nil {
		hc.notImplemented(c)
		return
	}
	accountID := c.Param("id")
	if err := hc.accounts.Delete(c.Request.Context(), accountID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		if errors.Is(err, db.ErrAccountInUse) {
			respondError(c, http.StatusConflict, CodeAccountInUse)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.deleteSearchDocument(c, "account_"+accountID)
	c.Status(http.StatusNoContent)
}

func accountResponseFromModel(account models.Account) accountResponse {
	return accountResponse{
		ID:        account.ID,
		EntityID:  account.EntityID,
		Name:      account.Name,
		Type:      account.Type,
		Code:      account.Code,
		RoleTags:  account.RoleTags,
		CreatedAt: account.CreatedAt.Format(time.RFC3339),
	}
}

func normalizeAccountRoleTags(roleTags []string) ([]string, bool) {
	if roleTags == nil {
		return nil, true
	}
	seen := make(map[string]struct{}, len(roleTags))
	out := make([]string, 0, len(roleTags))
	for _, roleTag := range roleTags {
		normalized := strings.TrimSpace(strings.ToLower(roleTag))
		normalized = strings.ReplaceAll(normalized, "-", "_")
		normalized = strings.Join(strings.Fields(normalized), "_")
		switch normalized {
		case models.AccountRoleUtilities, models.AccountRoleCellPhone, models.AccountRoleInternet:
		default:
			return nil, false
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, true
}

func (hc *HandlerContext) notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": CodeNotImplemented,
	})
}

func (hc *HandlerContext) issueSession(ctx context.Context, userID, tenantID string) (loginResponse, error) {
	token, err := hc.tokens.Issue(userID, tenantID, hc.jwtTTL)
	if err != nil {
		return loginResponse{}, err
	}
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return loginResponse{}, err
	}
	refreshHash := auth.HashRefreshToken(refreshToken)
	refreshExpiresAt := time.Now().Add(hc.refreshTTL)
	if _, err := hc.refreshTokens.Create(ctx, userID, tenantID, refreshHash, refreshExpiresAt); err != nil {
		return loginResponse{}, err
	}
	return loginResponse{
		Token:            token,
		TokenType:        "Bearer",
		ExpiresIn:        int64(hc.jwtTTL.Seconds()),
		RefreshToken:     refreshToken,
		RefreshExpiresIn: int64(hc.refreshTTL.Seconds()),
		TenantID:         tenantID,
	}, nil
}

func (hc *HandlerContext) registerUser(ctx context.Context, email, passwordHash, tenantName string) (string, string, error) {
	tenantName = strings.TrimSpace(tenantName)
	if tenantName == "" {
		tenantName = "Default Tenant"
	}

	if conn, ok := hc.ready.(*db.DB); ok && conn != nil {
		tx, err := conn.BeginTxx(ctx, nil)
		if err != nil {
			return "", "", err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		var tenantID string
		if err := tx.GetContext(ctx, &tenantID, `
			INSERT INTO tenants (name)
			VALUES ($1)
			RETURNING id
		`, tenantName); err != nil {
			return "", "", err
		}

		var userID string
		if err := tx.GetContext(ctx, &userID, `
			INSERT INTO users (email, password_hash, is_admin)
			VALUES ($1, $2, false)
			RETURNING id
		`, email, passwordHash); err != nil {
			if isDuplicateEmailErr(err) {
				return "", "", errEmailAlreadyExists
			}
			return "", "", err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenant_memberships (tenant_id, user_id, role)
			VALUES ($1, $2, $3)
		`, tenantID, userID, string(models.RoleUser)); err != nil {
			return "", "", err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET default_tenant_id = $2
			WHERE id = $1
		`, userID, tenantID); err != nil {
			return "", "", err
		}
		if err := tx.Commit(); err != nil {
			return "", "", err
		}
		return userID, tenantID, nil
	}

	user, err := hc.users.Create(ctx, email, passwordHash, false)
	if err != nil {
		if isDuplicateEmailErr(err) {
			return "", "", errEmailAlreadyExists
		}
		return "", "", err
	}
	tenant, err := hc.tenants.Create(ctx, tenantName)
	if err != nil {
		return "", "", err
	}
	if _, err := hc.tenantMembers.Create(ctx, tenant.ID, user.ID, models.RoleUser); err != nil {
		return "", "", err
	}
	if err := hc.users.SetDefaultTenant(ctx, user.ID, tenant.ID); err != nil {
		return "", "", err
	}
	return user.ID, tenant.ID, nil
}

func isDuplicateEmailErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return true
		}
	}
	return false
}

func (hc *HandlerContext) resolveTenantForLogin(ctx context.Context, userID, requestedTenantID string) (string, error) {
	requestedTenantID = strings.TrimSpace(requestedTenantID)
	if requestedTenantID != "" {
		if _, err := hc.tenantMembers.GetRole(ctx, requestedTenantID, userID); err != nil {
			return "", err
		}
		return requestedTenantID, nil
	}
	user, err := hc.users.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user.DefaultTenantID != "" {
		if _, err := hc.tenantMembers.GetRole(ctx, user.DefaultTenantID, userID); err == nil {
			return user.DefaultTenantID, nil
		}
	}
	memberships, err := hc.tenantMembers.ListForUser(ctx, userID, 1)
	if err != nil {
		return "", err
	}
	if len(memberships) == 0 {
		return "", db.ErrNotFound
	}
	return memberships[0].TenantID, nil
}

// defaultAccountDefs is the chart of accounts seeded when an entity is created without a template — the
// `basic` starter set. Falls back to a lone Cash account only if the basic template fails to load (so a
// new entity is never left with an empty, unusable chart).
func defaultAccountDefs() []templates.AccountDef {
	if tmpl, err := templates.Lookup(templates.DefaultKey); err == nil {
		return tmpl.Accounts
	}
	return []templates.AccountDef{
		{Name: "Cash", Type: "asset"},
	}
}

// resolveTemplateDefs turns a (possibly empty) template key into the account definitions to seed. An
// empty key means the default (basic) chart; an unknown key is an error so the API can reject it.
func resolveTemplateDefs(key string) ([]templates.AccountDef, error) {
	if strings.TrimSpace(key) == "" {
		return defaultAccountDefs(), nil
	}
	tmpl, err := templates.Lookup(key)
	if err != nil {
		return nil, err
	}
	return tmpl.Accounts, nil
}

// seedEntityAccounts creates the starter chart of accounts for a new entity (dedup by name, always with
// a Cash account for the pipeline's credit leg). Best-effort per-account, matching the create flow: a
// failed insert shouldn't abort entity creation.
func (hc *HandlerContext) seedEntityAccounts(ctx context.Context, entityID string, defs []templates.AccountDef) {
	if hc.accounts == nil {
		return
	}
	defs = ensureCashAccount(defs)
	seen := map[string]struct{}{}
	for _, def := range defs {
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		_, _ = hc.accounts.Create(ctx, entityID, def.Name, def.Type, def.Code)
	}
}

func ensureCashAccount(defs []templates.AccountDef) []templates.AccountDef {
	for _, def := range defs {
		if strings.EqualFold(strings.TrimSpace(def.Name), "cash") {
			return defs
		}
	}
	return append(defs, templates.AccountDef{Name: "Cash", Type: "asset"})
}

func adminRoles() map[models.Role]bool {
	return map[models.Role]bool{
		models.RoleAdmin: true,
	}
}

func memberRoles() map[models.Role]bool {
	return map[models.Role]bool{
		models.RoleAdmin:      true,
		models.RoleAccountant: true,
		models.RoleUser:       true,
	}
}
