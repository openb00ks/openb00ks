package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (hc *HandlerContext) cors() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(hc.corsAllowedOrigins))
	for _, origin := range hc.corsAllowedOrigins {
		val := strings.TrimSpace(origin)
		if val != "" {
			allowed[val] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}

		c.Next()
	}
}
