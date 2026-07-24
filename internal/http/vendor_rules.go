package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type vendorRuleRequest struct {
	EntityID  string `json:"entity_id"`
	MatchType string `json:"match_type"`
	Pattern   string `json:"pattern"`
	AccountID string `json:"account_id"`
}

func (hc *HandlerContext) handleVendorRulesList(c *gin.Context) {
	if hc.vendorRules == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	limit := queryLimit(c, 200, 1000)
	rows, err := hc.vendorRules.List(c.Request.Context(), entityID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, vendorRuleResponse(row))
	}
	c.JSON(http.StatusOK, gin.H{"rows": resp})
}

func (hc *HandlerContext) handleVendorRulesCreate(c *gin.Context) {
	if hc.vendorRules == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req vendorRuleRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || req.MatchType == "" || req.Pattern == "" || req.AccountID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if req.MatchType != "exact" && req.MatchType != "contains" {
		respondError(c, http.StatusBadRequest, CodeInvalidMatchType)
		return
	}
	rule, err := hc.vendorRules.Create(c.Request.Context(), models.VendorRule{
		EntityID:  req.EntityID,
		MatchType: req.MatchType,
		Pattern:   req.Pattern,
		AccountID: req.AccountID,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusCreated, vendorRuleResponse(rule))
}

func (hc *HandlerContext) handleVendorRulesUpdate(c *gin.Context) {
	if hc.vendorRules == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req vendorRuleRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.MatchType == "" || req.Pattern == "" || req.AccountID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if req.MatchType != "exact" && req.MatchType != "contains" {
		respondError(c, http.StatusBadRequest, CodeInvalidMatchType)
		return
	}
	rule, err := hc.vendorRules.Update(c.Request.Context(), c.Param("id"), models.VendorRule{
		EntityID:  req.EntityID,
		MatchType: req.MatchType,
		Pattern:   req.Pattern,
		AccountID: req.AccountID,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, vendorRuleResponse(rule))
}

func (hc *HandlerContext) handleVendorRulesDelete(c *gin.Context) {
	if hc.vendorRules == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	if err := hc.vendorRules.Delete(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

func vendorRuleResponse(rule models.VendorRule) gin.H {
	return gin.H{
		"id":         rule.ID,
		"entity_id":  rule.EntityID,
		"match_type": rule.MatchType,
		"pattern":    rule.Pattern,
		"account_id": rule.AccountID,
		"created_at": rule.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
