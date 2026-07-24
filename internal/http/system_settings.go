package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type systemSettingsData struct {
	RequireMFA            bool `json:"require_mfa"`
	EnforceSessionTimeout bool `json:"enforce_session_timeout"`
}

type systemSettingsResponse struct {
	Settings     systemSettingsData `json:"settings"`
	Integrations systemIntegrations `json:"integrations"`
	UpdatedAt    string             `json:"updated_at,omitempty"`
}

type systemIntegrations struct {
	AIProvider      string `json:"ai_provider"`
	AIModel         string `json:"ai_model"`
	ReceiptStorage  string `json:"receipt_storage"`
	ReceiptLocalDir string `json:"receipt_local_dir"`
	ReceiptMaxBytes int64  `json:"receipt_max_bytes"`
}

type systemSettingsUpdateRequest struct {
	RequireMFA            *bool `json:"require_mfa"`
	EnforceSessionTimeout *bool `json:"enforce_session_timeout"`
}

func (hc *HandlerContext) handleSystemSettingsGet(c *gin.Context) {
	if hc.systemSettings == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	settings, err := hc.systemSettings.Get(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	payload := systemSettingsData{}
	if len(settings.SettingsJSON) > 0 {
		_ = json.Unmarshal(settings.SettingsJSON, &payload)
	}
	resp := systemSettingsResponse{
		Settings: payload,
		Integrations: systemIntegrations{
			AIProvider:      hc.systemInfo.AIProvider,
			AIModel:         hc.systemInfo.AIModel,
			ReceiptStorage:  hc.systemInfo.ReceiptStorage,
			ReceiptLocalDir: hc.systemInfo.ReceiptLocalDir,
			ReceiptMaxBytes: hc.systemInfo.ReceiptMaxBytes,
		},
	}
	if !settings.UpdatedAt.IsZero() {
		resp.UpdatedAt = settings.UpdatedAt.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleSystemSettingsUpdate(c *gin.Context) {
	if hc.systemSettings == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req systemSettingsUpdateRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	current := systemSettingsData{}
	settings, err := hc.systemSettings.Get(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if len(settings.SettingsJSON) > 0 {
		_ = json.Unmarshal(settings.SettingsJSON, &current)
	}
	if req.RequireMFA != nil {
		current.RequireMFA = *req.RequireMFA
	}
	if req.EnforceSessionTimeout != nil {
		current.EnforceSessionTimeout = *req.EnforceSessionTimeout
	}
	nextJSON, err := json.Marshal(current)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	updated, err := hc.systemSettings.UpsertSettings(c.Request.Context(), nextJSON)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := systemSettingsResponse{
		Settings: current,
		Integrations: systemIntegrations{
			AIProvider:      hc.systemInfo.AIProvider,
			AIModel:         hc.systemInfo.AIModel,
			ReceiptStorage:  hc.systemInfo.ReceiptStorage,
			ReceiptLocalDir: hc.systemInfo.ReceiptLocalDir,
			ReceiptMaxBytes: hc.systemInfo.ReceiptMaxBytes,
		},
	}
	if !updated.UpdatedAt.IsZero() {
		resp.UpdatedAt = updated.UpdatedAt.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, resp)
}
