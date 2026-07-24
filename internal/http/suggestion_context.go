package httpapi

import "strings"

func formSuggestionContext(ctxValue, legacyValue string) string {
	if value := strings.TrimSpace(ctxValue); value != "" {
		return value
	}
	return strings.TrimSpace(legacyValue)
}
