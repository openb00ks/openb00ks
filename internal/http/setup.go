package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type setupRequest struct {
	TenantName        string `json:"tenant_name"`
	AdminEmail        string `json:"admin_email"`
	AdminPassword     string `json:"admin_password"`
	DefaultEntityName string `json:"default_entity_name"`
	Template          string `json:"template"`
}

type setupResponse struct {
	TenantID    string `json:"tenant_id"`
	AdminUserID string `json:"admin_user_id"`
	EntityID    string `json:"entity_id,omitempty"`
}

func (hc *HandlerContext) setupRequired(ctx *gin.Context) (bool, error) {
	if hc.systemSettings == nil {
		return false, db.ErrUnavailable
	}
	settings, err := hc.systemSettings.Get(ctx.Request.Context())
	if err != nil {
		return false, err
	}
	return !settings.SetupComplete, nil
}

func (hc *HandlerContext) handleSetupStatus(c *gin.Context) {
	required, err := hc.setupRequired(c)
	if err != nil {
		if errors.Is(err, db.ErrUnavailable) {
			respondError(c, http.StatusNotImplemented, CodeNotImplemented)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, SetupStatus{Required: required})
}

func (hc *HandlerContext) handleSetup(c *gin.Context) {
	if hc.users == nil || hc.tenants == nil || hc.tenantMembers == nil || hc.systemSettings == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	required, err := hc.setupRequired(c)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if !required {
		respondError(c, http.StatusConflict, CodeSetupAlreadyComplete)
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	tenantName := strings.TrimSpace(req.TenantName)
	if tenantName == "" {
		tenantName = "Default Tenant"
	}
	email := strings.TrimSpace(req.AdminEmail)
	password := req.AdminPassword
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

	tenant, err := hc.tenants.Create(c.Request.Context(), tenantName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	user, err := hc.users.Create(c.Request.Context(), email, hash, true)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if _, err := hc.tenantMembers.Create(c.Request.Context(), tenant.ID, user.ID, models.RoleAdmin); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if err := hc.users.SetDefaultTenant(c.Request.Context(), user.ID, tenant.ID); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	entityID := ""
	if hc.entities != nil {
		entityName := strings.TrimSpace(req.DefaultEntityName)
		if entityName != "" {
			entity, err := hc.entities.CreateWithOwner(c.Request.Context(), tenant.ID, user.ID, entityName, "")
			if err == nil {
				entityID = entity.ID
				// Seed the starter chart of accounts (unknown template → default), so the day-one
				// entity is usable — previously setup created the entity with no accounts.
				defs, _ := resolveTemplateDefs(req.Template)
				hc.seedEntityAccounts(c.Request.Context(), entity.ID, defs)
			}
		}
	}

	if _, err := hc.systemSettings.SetSetupComplete(c.Request.Context(), time.Now().UTC()); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	c.JSON(http.StatusCreated, setupResponse{
		TenantID:    tenant.ID,
		AdminUserID: user.ID,
		EntityID:    entityID,
	})
}
