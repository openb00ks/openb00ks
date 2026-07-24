package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/migrate"
)

func (hc *HandlerContext) handleMigrationStatus(c *gin.Context) {
	dbURL := hc.migrationDBURL()
	if dbURL == "" {
		respondError(c, http.StatusServiceUnavailable, CodeDbUnavailable)
		return
	}
	version, dirty, err := migrate.Version(dbURL)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"version": version,
		"dirty":   dirty,
	})
}

func (hc *HandlerContext) handleMigrationUp(c *gin.Context) {
	dbURL := hc.migrationDBURL()
	if dbURL == "" {
		respondError(c, http.StatusServiceUnavailable, CodeDbUnavailable)
		return
	}
	if err := migrate.Up(dbURL, 0); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	version, dirty, err := migrate.Version(dbURL)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"version":    version,
		"dirty":      dirty,
		"applied_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (hc *HandlerContext) migrationDBURL() string {
	if hc.ready == nil {
		return ""
	}
	if provider, ok := hc.ready.(interface{ DSN() string }); ok {
		return provider.DSN()
	}
	return ""
}
