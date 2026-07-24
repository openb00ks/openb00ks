package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
)

type updatePreferencesRequest struct {
	DefaultEntityID string `json:"default_entity_id"`
	Theme           string `json:"theme"`
}

func (hc *HandlerContext) handlePreferencesGet(c *gin.Context) {
	if hc.preferences == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)
	prefs, err := hc.preferences.Get(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"user_id":           userID,
				"default_entity_id": "",
				"theme":             "system",
			})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":           prefs.UserID,
		"default_entity_id": prefs.DefaultEntityID,
		"theme":             prefs.Theme,
		"created_at":        prefs.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":        prefs.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (hc *HandlerContext) handlePreferencesUpdate(c *gin.Context) {
	if hc.preferences == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)
	var req updatePreferencesRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Theme == "" {
		req.Theme = "system"
	}
	switch req.Theme {
	case "light", "dark", "system":
	default:
		respondError(c, http.StatusBadRequest, CodeInvalidTheme)
		return
	}

	prefs, err := hc.preferences.Upsert(c.Request.Context(), userID, req.DefaultEntityID, req.Theme)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if req.DefaultEntityID != "" {
		hc.auditEvent(c.Request.Context(), req.DefaultEntityID, userID, "preferences", prefs.UserID, "preferences.updated", nil, map[string]interface{}{
			"default_entity_id": prefs.DefaultEntityID,
			"theme":             prefs.Theme,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":           prefs.UserID,
		"default_entity_id": prefs.DefaultEntityID,
		"theme":             prefs.Theme,
		"created_at":        prefs.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":        prefs.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
