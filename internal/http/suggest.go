package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type suggestRequest struct {
	ReceiptID string `json:"receipt_id"`
	Text      string `json:"text"`
	Extracted string `json:"extracted"`
	Context   string `json:"context"`
}

func (hc *HandlerContext) handleSuggest(c *gin.Context) {
	if hc.receiptStore == nil || hc.suggestions == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}

	var req suggestRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.ReceiptID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	suggestion, err := hc.suggestions.LatestByReceiptID(c.Request.Context(), req.ReceiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			fallback, fallbackErr := hc.suggestFromVendorRules(c, req)
			if fallbackErr != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
			if fallback != nil {
				hc.metrics.suggestionServed(c.Request.Context())
				c.JSON(http.StatusOK, fallback)
				return
			}
			respondError(c, http.StatusNotFound, CodeNoSuggestion)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	entityID, accountID, explanation := summarizeStoredSuggestion(suggestion.ParsedJSON)
	resp := gin.H{
		"receipt_id":    req.ReceiptID,
		"suggestion_id": suggestion.ID,
		"status":        suggestion.Status,
		"confidence":    suggestion.Confidence,
		"source":        "stored_receipt_suggestion",
		"created_at":    suggestion.CreatedAt.Format(time.RFC3339),
	}
	if entityID != "" {
		resp["entity_id"] = entityID
	}
	if accountID != "" {
		resp["account_id"] = accountID
	}
	if explanation != "" {
		resp["explanation"] = explanation
	}
	if len(suggestion.RawJSON) > 0 {
		resp["raw_payload"] = suggestion.RawJSON
	}

	hc.metrics.suggestionServed(c.Request.Context())
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) suggestFromVendorRules(c *gin.Context, req suggestRequest) (gin.H, error) {
	if hc.vendorRules == nil {
		return nil, nil
	}

	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), req.ReceiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	roleHints := []string{}
	if hc.accounts != nil {
		if accountRows, err := hc.accounts.ListForEntity(c.Request.Context(), receipt.EntityID, 1000); err == nil {
			roleHints = accountRoleHintTokens(accountRows)
		}
	}

	for _, candidate := range fallbackSuggestionCandidates(req, receipt.OriginalName, roleHints...) {
		matches, err := hc.vendorRules.FindMatching(c.Request.Context(), receipt.EntityID, candidate)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			continue
		}

		rule := matches[0]
		confidence := 0.56
		if rule.MatchType == "exact" {
			confidence = 0.74
		}
		return gin.H{
			"receipt_id":  req.ReceiptID,
			"entity_id":   receipt.EntityID,
			"account_id":  rule.AccountID,
			"confidence":  confidence,
			"source":      "vendor_rule_fallback",
			"explanation": "Matched vendor rule \"" + rule.Pattern + "\".",
			"raw_payload": gin.H{
				"rule_id":      rule.ID,
				"match_type":   rule.MatchType,
				"pattern":      rule.Pattern,
				"matched_text": candidate,
			},
		}, nil
	}

	return nil, nil
}

func fallbackSuggestionCandidates(req suggestRequest, originalName string, extras ...string) []string {
	raw := []string{req.Text, req.Extracted, req.Context, originalName}
	raw = append(raw, extras...)
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, candidate := range raw {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func accountRoleHintTokens(accounts []models.Account) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(accounts))
	for _, account := range accounts {
		for _, tag := range account.RoleTags {
			hint := strings.TrimSpace(tag)
			if hint == "" {
				continue
			}
			if _, ok := seen[hint]; ok {
				continue
			}
			seen[hint] = struct{}{}
			out = append(out, hint)
		}
	}
	sort.Strings(out)
	return out
}

func summarizeStoredSuggestion(parsed json.RawMessage) (string, string, string) {
	if len(parsed) == 0 {
		return "", "", ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(parsed, &payload); err != nil {
		return "", "", ""
	}

	entityID := firstString(payload, "entity_id", "entityId")
	accountID := firstString(payload, "account_id", "accountId")
	explanation := firstString(payload, "explanation", "reason")

	if nested, ok := payload["entity"].(map[string]interface{}); ok && entityID == "" {
		entityID = firstString(nested, "id", "entity_id")
	}
	if accountID == "" {
		if nested, ok := payload["account"].(map[string]interface{}); ok {
			accountID = firstString(nested, "id", "account_id")
		}
	}
	if accountID == "" {
		if entries, ok := payload["entries"].([]interface{}); ok && len(entries) > 0 {
			if first, ok := entries[0].(map[string]interface{}); ok {
				accountID = firstString(first, "account_id", "accountId")
			}
		}
	}

	return entityID, accountID, explanation
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if ok && value != "" {
			return value
		}
	}
	return ""
}
