package httpapi

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// queryLimit reads the "limit" query parameter, accepting values in (0, max].
// It returns def when the parameter is absent, non-numeric, or out of range.
func queryLimit(c *gin.Context, def, max int) int {
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= max {
			return n
		}
	}
	return def
}
