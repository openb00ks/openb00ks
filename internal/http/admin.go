package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (hc *HandlerContext) handleAdminStats(c *gin.Context) {
	if hc.adminStats == nil {
		hc.notImplemented(c)
		return
	}
	stats, err := hc.adminStats.Query(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (hc *HandlerContext) handleAdminJobsList(c *gin.Context) {
	if hc.receiptJobs == nil {
		hc.notImplemented(c)
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	stage := strings.TrimSpace(c.Query("stage"))
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)

	jobs, err := hc.receiptJobs.List(c.Request.Context(), status, stage, limit, offset)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs, "limit": limit, "offset": offset})
}

func (hc *HandlerContext) handleAdminJobRequeue(c *gin.Context) {
	if hc.queue == nil {
		hc.notImplemented(c)
		return
	}
	jobID := c.Param("id")
	if err := hc.queue.Requeue(c.Request.Context(), jobID); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleAdminErrorsList(c *gin.Context) {
	if hc.errors == nil {
		hc.notImplemented(c)
		return
	}
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)

	var resolved *bool
	if r := c.Query("resolved"); r != "" {
		v := r == "true"
		resolved = &v
	}

	errs, err := hc.errors.List(c.Request.Context(), resolved, limit, offset)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"errors": errs, "limit": limit, "offset": offset})
}

type resolveErrorRequest struct {
	Note string `json:"note"`
}

func (hc *HandlerContext) handleAdminErrorResolve(c *gin.Context) {
	if hc.errors == nil {
		hc.notImplemented(c)
		return
	}
	id := c.Param("id")
	var req resolveErrorRequest
	_ = c.ShouldBindJSON(&req)

	updated, err := hc.errors.Resolve(c.Request.Context(), id, strings.TrimSpace(req.Note), time.Now().UTC())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func queryInt(c *gin.Context, key string, defaultVal int) int {
	v := c.Query(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}
