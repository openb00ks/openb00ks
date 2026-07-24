package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/auth"
)

type contextKey string

const userIDKey contextKey = "user_id"
const tenantIDKey contextKey = "tenant_id"

func AuthRequired(tokens *auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			unauthorized(c)
			return
		}
		if tokens == nil {
			unauthorized(c)
			return
		}
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := tokens.Parse(tokenString)
		if err != nil {
			unauthorized(c)
			return
		}
		if claims.TenantID == "" {
			unauthorized(c)
			return
		}
		c.Set(string(userIDKey), claims.UserID)
		c.Set(string(tenantIDKey), claims.TenantID)
		c.Next()
	}
}

func UserID(c *gin.Context) (string, bool) {
	val, ok := c.Get(string(userIDKey))
	if !ok {
		return "", false
	}
	userID, ok := val.(string)
	return userID, ok
}

func TenantID(c *gin.Context) (string, bool) {
	val, ok := c.Get(string(tenantIDKey))
	if !ok {
		return "", false
	}
	tenantID, ok := val.(string)
	return tenantID, ok
}

func AdminRequired(admin AdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserID(c)
		if !ok || admin == nil {
			unauthorized(c)
			return
		}
		isAdmin, err := admin.IsAdmin(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": CodeInternalError,
			})
			return
		}
		if !isAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": CodeForbidden,
			})
			return
		}
		c.Next()
	}
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": CodeUnauthorized,
	})
}
