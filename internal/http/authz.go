package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type entityResolver func(*gin.Context) (string, error)

type receiptIDsResolver func(*gin.Context) ([]string, error)

const entityIDKey contextKey = "entity_id"
const entityRoleKey contextKey = "entity_role"
const tenantRoleKey contextKey = "tenant_role"
const targetTenantIDKey contextKey = "target_tenant_id"

func (hc *HandlerContext) requireTenantMembership() gin.HandlerFunc {
	return func(c *gin.Context) {
		if hc.tenantMembers == nil {
			c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": CodeNotImplemented})
			return
		}
		userID, ok := UserID(c)
		if !ok {
			unauthorized(c)
			return
		}
		tenantID, ok := TenantID(c)
		if !ok || tenantID == "" {
			unauthorized(c)
			return
		}
		role, err := hc.tenantMembers.GetRole(c.Request.Context(), tenantID, userID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
			return
		}
		c.Set(string(tenantRoleKey), role)
		c.Next()
	}
}

func (hc *HandlerContext) requireTenantAccess(param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hc.tenantMembers == nil {
			c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": CodeNotImplemented})
			return
		}
		userID, ok := UserID(c)
		if !ok {
			unauthorized(c)
			return
		}
		tenantID := c.Param(param)
		if tenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": CodeMissingFields})
			return
		}
		if _, err := hc.tenantMembers.GetRole(c.Request.Context(), tenantID, userID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
			return
		}
		c.Set(string(targetTenantIDKey), tenantID)
		c.Next()
	}
}

func (hc *HandlerContext) requireEntityRole(allowed map[models.Role]bool, resolve entityResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hc.entities == nil {
			c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": CodeNotImplemented})
			return
		}
		userID, ok := UserID(c)
		if !ok {
			unauthorized(c)
			return
		}
		tenantID, ok := TenantID(c)
		if !ok || tenantID == "" {
			unauthorized(c)
			return
		}
		entityID, err := resolve(c)
		if err != nil {
			handleEntityResolveError(c, err)
			return
		}
		role, err := hc.entities.GetRole(c.Request.Context(), tenantID, userID, entityID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
			return
		}
		if !allowed[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
			return
		}
		c.Set(string(entityIDKey), entityID)
		c.Set(string(entityRoleKey), role)
		c.Next()
	}
}

func (hc *HandlerContext) requireOptionalEntityRole(allowed map[models.Role]bool, field string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hc.entities == nil {
			c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": CodeNotImplemented})
			return
		}
		userID, ok := UserID(c)
		if !ok {
			unauthorized(c)
			return
		}
		tenantID, ok := TenantID(c)
		if !ok || tenantID == "" {
			unauthorized(c)
			return
		}
		var payload map[string]string
		if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": CodeBadRequest})
			return
		}
		entityID := payload[field]
		if entityID == "" {
			c.Next()
			return
		}
		role, err := hc.entities.GetRole(c.Request.Context(), tenantID, userID, entityID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
			return
		}
		if !allowed[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": CodeForbidden})
			return
		}
		c.Set(string(entityIDKey), entityID)
		c.Set(string(entityRoleKey), role)
		c.Next()
	}
}

func (hc *HandlerContext) requireReceiptsRole(allowed map[models.Role]bool, resolve receiptIDsResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hc.entities == nil || hc.receiptStore == nil {
			c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": CodeNotImplemented})
			return
		}
		userID, ok := UserID(c)
		if !ok {
			unauthorized(c)
			return
		}
		tenantID, ok := TenantID(c)
		if !ok || tenantID == "" {
			unauthorized(c)
			return
		}
		receiptIDs, err := resolve(c)
		if err != nil {
			handleEntityResolveError(c, err)
			return
		}
		allowedReceipts := make([]string, 0, len(receiptIDs))
		for _, receiptID := range receiptIDs {
			entityID, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					continue
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
				return
			}
			role, err := hc.entities.GetRole(c.Request.Context(), tenantID, userID, entityID)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					continue
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
				return
			}
			if !allowed[role] {
				continue
			}
			allowedReceipts = append(allowedReceipts, receiptID)
		}
		c.Set(string(entityIDKey), allowedReceipts)
		c.Next()
	}
}

func handleEntityResolveError(c *gin.Context, err error) {
	if errors.Is(err, db.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": CodeNotFound})
		return
	}
	if errors.Is(err, errMissingEntityID) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": CodeMissingFields})
		return
	}
	if errors.Is(err, errBadRequest) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": CodeBadRequest})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": CodeInternalError})
}

var errMissingEntityID = errors.New("missing entity id")
var errBadRequest = errors.New("bad request")

func entityIDFromParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		val := c.Param(name)
		if val == "" {
			return "", errMissingEntityID
		}
		return val, nil
	}
}

func entityIDFromQuery(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		val := c.Query(name)
		if val == "" {
			return "", errMissingEntityID
		}
		return val, nil
	}
}

func entityIDFromForm(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		val := c.PostForm(name)
		if val == "" {
			return "", errMissingEntityID
		}
		return val, nil
	}
}

func entityIDFromJSON(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		var payload map[string]interface{}
		if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
			return "", errBadRequest
		}
		raw, ok := payload[name]
		if !ok {
			return "", errMissingEntityID
		}
		val, ok := raw.(string)
		if !ok || val == "" {
			return "", errMissingEntityID
		}
		return val, nil
	}
}

func (hc *HandlerContext) entityIDFromAccountParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.accounts == nil {
			return "", db.ErrUnavailable
		}
		accountID := c.Param(name)
		if accountID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.accounts.GetEntityID(c.Request.Context(), accountID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromAccountStatementParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.accountStatements == nil {
			return "", db.ErrUnavailable
		}
		statementID := c.Param(name)
		if statementID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.accountStatements.GetEntityID(c.Request.Context(), statementID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromReceiptParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.receiptStore == nil {
			return "", db.ErrUnavailable
		}
		receiptID := c.Param(name)
		if receiptID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromReceiptJSON(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.receiptStore == nil {
			return "", db.ErrUnavailable
		}
		var payload map[string]interface{}
		if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
			return "", errBadRequest
		}
		raw, ok := payload[name]
		if !ok {
			return "", errMissingEntityID
		}
		receiptID, ok := raw.(string)
		if !ok || receiptID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromVendorRuleParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.vendorRules == nil {
			return "", db.ErrUnavailable
		}
		ruleID := c.Param(name)
		if ruleID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.vendorRules.GetEntityID(c.Request.Context(), ruleID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromVendorParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.vendors == nil {
			return "", db.ErrUnavailable
		}
		vendorID := c.Param(name)
		if vendorID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.vendors.GetEntityID(c.Request.Context(), vendorID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func (hc *HandlerContext) entityIDFromMileageParam(name string) entityResolver {
	return func(c *gin.Context) (string, error) {
		if hc.mileage == nil {
			return "", db.ErrUnavailable
		}
		mileageID := c.Param(name)
		if mileageID == "" {
			return "", errMissingEntityID
		}
		entityID, err := hc.mileage.GetEntityID(c.Request.Context(), mileageID)
		if err != nil {
			return "", err
		}
		return entityID, nil
	}
}

func receiptIDsFromBody(name string) receiptIDsResolver {
	return func(c *gin.Context) ([]string, error) {
		var payload map[string][]string
		if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
			return nil, errBadRequest
		}
		ids := payload[name]
		if len(ids) == 0 {
			return nil, errMissingEntityID
		}
		return ids, nil
	}
}

func EntityID(c *gin.Context) (string, bool) {
	val, ok := c.Get(string(entityIDKey))
	if !ok {
		return "", false
	}
	entityID, ok := val.(string)
	return entityID, ok
}

func EntityIDs(c *gin.Context) ([]string, bool) {
	val, ok := c.Get(string(entityIDKey))
	if !ok {
		return nil, false
	}
	entityIDs, ok := val.([]string)
	return entityIDs, ok
}

func EntityRole(c *gin.Context) (models.Role, bool) {
	val, ok := c.Get(string(entityRoleKey))
	if !ok {
		return "", false
	}
	role, ok := val.(models.Role)
	return role, ok
}

func TargetTenantID(c *gin.Context) (string, bool) {
	val, ok := c.Get(string(targetTenantIDKey))
	if !ok {
		return "", false
	}
	tenantID, ok := val.(string)
	return tenantID, ok
}

func userIDFromContext(c *gin.Context) string {
	userID, _ := UserID(c)
	return userID
}

func tenantIDFromContext(c *gin.Context) string {
	tenantID, _ := TenantID(c)
	return tenantID
}
