package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
)

type mfaStatusResponse struct {
	Configured    bool     `json:"configured"`
	Enabled       bool     `json:"enabled"`
	Secret        string   `json:"secret,omitempty"`
	URI           string   `json:"uri,omitempty"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

type mfaConfirmRequest struct {
	Code string `json:"code"`
}

func (hc *HandlerContext) systemRequiresMFA(ctx *gin.Context) (bool, error) {
	if hc.systemSettings == nil {
		return false, db.ErrUnavailable
	}
	settings, err := hc.systemSettings.Get(ctx.Request.Context())
	if err != nil {
		return false, err
	}
	var payload systemSettingsData
	if len(settings.SettingsJSON) > 0 {
		_ = json.Unmarshal(settings.SettingsJSON, &payload)
	}
	return payload.RequireMFA, nil
}

func (hc *HandlerContext) handleLoginMFA(c *gin.Context) {
	if hc.tokens == nil || hc.refreshTokens == nil || hc.users == nil || hc.tenantMembers == nil || hc.userMFA == nil || hc.usedTOTPSteps == nil {
		hc.notImplemented(c)
		return
	}
	var req loginMFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.ChallengeToken == "" || req.Code == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	claims, err := hc.tokens.ParseChallenge(req.ChallengeToken, auth.MFATokenPurpose)
	if err != nil {
		if errors.Is(err, auth.ErrExpiredToken) {
			respondError(c, http.StatusUnauthorized, CodeMfaChallengeExpired)
			return
		}
		respondError(c, http.StatusUnauthorized, CodeInvalidMfaChallenge)
		return
	}
	record, err := hc.userMFA.GetByUserID(c.Request.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusConflict, CodeMfaSetupRequired)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if !record.Enabled || record.Secret == "" {
		respondError(c, http.StatusConflict, CodeMfaSetupRequired)
		return
	}
	now := time.Now().UTC()
	if step, ok := auth.ValidateTOTPStep(record.Secret, req.Code, now); ok {
		if err := hc.usedTOTPSteps.MarkUsed(c.Request.Context(), claims.UserID, step, now); err != nil {
			if errors.Is(err, db.ErrConflict) {
				respondError(c, http.StatusUnauthorized, CodeMfaCodeAlreadyUsed)
				return
			}
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		resp, err := hc.issueSession(c.Request.Context(), claims.UserID, claims.TenantID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	hashes := mfaRecoveryHashes(record)
	hash := auth.HashRecoveryCode(req.Code)
	if !containsString(hashes, hash) {
		respondError(c, http.StatusUnauthorized, CodeInvalidMfaCode)
		return
	}
	next := removeString(hashes, hash)
	nextJSON, err := json.Marshal(next)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if _, err := hc.userMFA.SetRecoveryCodeHashes(c.Request.Context(), claims.UserID, nextJSON); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp, err := hc.issueSession(c.Request.Context(), claims.UserID, claims.TenantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleMFAStatus(c *gin.Context) {
	userID, ok := UserID(c)
	if !ok || hc.userMFA == nil {
		hc.notImplemented(c)
		return
	}
	record, err := hc.userMFA.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusOK, mfaStatusResponse{})
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, mfaStatusResponse{
		Configured: record.Secret != "",
		Enabled:    record.Enabled,
	})
}

func (hc *HandlerContext) handleMFASetup(c *gin.Context) {
	userID, ok := UserID(c)
	if !ok || hc.userMFA == nil {
		hc.notImplemented(c)
		return
	}
	user, err := hc.users.GetByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	_, err = hc.userMFA.GetByUserID(c.Request.Context(), userID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	secret, err := auth.GenerateMFASecret()
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	recoveryCodes, recoveryHashes, err := auth.GenerateRecoveryCodes(0)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	recoveryJSON, err := json.Marshal(recoveryHashes)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if _, err := hc.userMFA.UpsertEnrollment(c.Request.Context(), userID, secret, recoveryJSON); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, mfaStatusResponse{
		Configured:    true,
		Enabled:       false,
		Secret:        secret,
		URI:           auth.BuildMFAProvisioningURI(secret, user.Email),
		RecoveryCodes: recoveryCodes,
	})
}

func (hc *HandlerContext) handleMFAConfirm(c *gin.Context) {
	userID, ok := UserID(c)
	if !ok || hc.userMFA == nil || hc.usedTOTPSteps == nil {
		hc.notImplemented(c)
		return
	}
	var req mfaConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Code == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
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
	now := time.Now().UTC()
	step, ok := auth.ValidateTOTPStep(record.Secret, strings.TrimSpace(req.Code), now)
	if !ok {
		respondError(c, http.StatusUnauthorized, CodeInvalidMfaCode)
		return
	}
	if err := hc.usedTOTPSteps.MarkUsed(c.Request.Context(), userID, step, now); err != nil {
		if errors.Is(err, db.ErrConflict) {
			respondError(c, http.StatusUnauthorized, CodeMfaCodeAlreadyUsed)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if _, err := hc.userMFA.Enable(c.Request.Context(), userID); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, mfaStatusResponse{
		Configured: true,
		Enabled:    true,
	})
}

func mfaRecoveryHashes(record db.UserMFA) []string {
	if len(record.RecoveryCodeHashes) == 0 {
		return nil
	}
	var hashes []string
	_ = json.Unmarshal(record.RecoveryCodeHashes, &hashes)
	return hashes
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

type mfaDisableRequest struct {
	Code string `json:"code"`
}

func (hc *HandlerContext) handleMFADisable(c *gin.Context) {
	userID, ok := UserID(c)
	if !ok || hc.userMFA == nil {
		hc.notImplemented(c)
		return
	}
	var req mfaDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	record, err := hc.userMFA.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if !record.Enabled || record.Secret == "" {
		c.Status(http.StatusNoContent)
		return
	}
	if !auth.ValidateTOTP(record.Secret, req.Code, time.Now().UTC()) {
		respondError(c, http.StatusUnauthorized, CodeInvalidMfaCode)
		return
	}
	if _, err := hc.userMFA.Disable(c.Request.Context(), userID); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}
