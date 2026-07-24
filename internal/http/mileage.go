package httpapi

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type mileageRequest struct {
	EntityID          string  `json:"entity_id"`
	Date              string  `json:"date"`
	DistanceMiles     float64 `json:"distance_miles"`
	StartLocation     string  `json:"start_location"`
	EndLocation       string  `json:"end_location"`
	Purpose           string  `json:"purpose"`
	ReceiptID         string  `json:"receipt_id"`
	Context           string  `json:"context"`
	SuggestionContext string  `json:"suggestion_context"`
}

func (hc *HandlerContext) handleMileageCreate(c *gin.Context) {
	if hc.mileage == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req mileageRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || req.Date == "" || req.DistanceMiles <= 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}
	log, err := hc.mileage.Create(c.Request.Context(), models.MileageLog{
		EntityID:      req.EntityID,
		UserID:        userIDFromContext(c),
		ReceiptID:     req.ReceiptID,
		Date:          date,
		DistanceMiles: req.DistanceMiles,
		StartLocation: req.StartLocation,
		EndLocation:   req.EndLocation,
		Purpose:       req.Purpose,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	ctxText := formSuggestionContext(req.SuggestionContext, req.Context)
	if hc.mileageMeta != nil && ctxText != "" {
		_ = hc.mileageMeta.UpsertSuggestionContext(c.Request.Context(), log.ID, ctxText)
	}
	hc.auditEvent(c.Request.Context(), req.EntityID, userIDFromContext(c), "mileage", log.ID, "mileage.created", nil, map[string]interface{}{
		"id":             log.ID,
		"date":           log.Date.Format("2006-01-02"),
		"distance_miles": log.DistanceMiles,
	})
	hc.indexMileage(c, log, ctxText)
	c.JSON(http.StatusCreated, mileageResponse(log, ctxText))
}

func (hc *HandlerContext) handleMileageUpdate(c *gin.Context) {
	if hc.mileage == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req mileageRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || req.Date == "" || req.DistanceMiles <= 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}
	id := c.Param("id")
	log, err := hc.mileage.Update(c.Request.Context(), id, models.MileageLog{
		Date:          date,
		DistanceMiles: req.DistanceMiles,
		StartLocation: req.StartLocation,
		EndLocation:   req.EndLocation,
		Purpose:       req.Purpose,
		ReceiptID:     req.ReceiptID,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	ctxText := formSuggestionContext(req.SuggestionContext, req.Context)
	if hc.mileageMeta != nil && ctxText != "" {
		_ = hc.mileageMeta.UpsertSuggestionContext(c.Request.Context(), log.ID, ctxText)
	}
	hc.auditEvent(c.Request.Context(), req.EntityID, userIDFromContext(c), "mileage", log.ID, "mileage.updated", nil, map[string]interface{}{
		"id":             log.ID,
		"date":           log.Date.Format("2006-01-02"),
		"distance_miles": log.DistanceMiles,
	})
	hc.indexMileage(c, log, ctxText)
	c.JSON(http.StatusOK, mileageResponse(log, ctxText))
}

func (hc *HandlerContext) handleMileageDelete(c *gin.Context) {
	if hc.mileage == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	id := c.Param("id")
	var entityID string
	if hc.entities != nil {
		if log, err := hc.mileage.GetByID(c.Request.Context(), id); err == nil {
			entityID = log.EntityID
		}
	}
	if err := hc.mileage.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if entityID != "" {
		hc.auditEvent(c.Request.Context(), entityID, userIDFromContext(c), "mileage", id, "mileage.deleted", nil, map[string]interface{}{
			"id": id,
		})
	}
	hc.deleteSearchDocument(c, "mileage_"+id)
	c.Status(http.StatusNoContent)
}

func (hc *HandlerContext) handleMileageList(c *gin.Context) {
	if hc.mileage == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	var start *time.Time
	var end *time.Time
	if v := c.Query("start_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			start = &t
		} else {
			respondError(c, http.StatusBadRequest, CodeInvalidDate)
			return
		}
	}
	if v := c.Query("end_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end = &t
		} else {
			respondError(c, http.StatusBadRequest, CodeInvalidDate)
			return
		}
	}
	limit := queryLimit(c, 100, 1000)

	logs, err := hc.mileage.List(c.Request.Context(), entityID, start, end, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		ctxText := ""
		if hc.mileageMeta != nil {
			if stored, err := hc.mileageMeta.GetSuggestionContext(c.Request.Context(), log.ID); err == nil {
				ctxText = stored
			}
		}
		resp = append(resp, mileageResponse(log, ctxText))
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"rows":      resp,
	})
}

func (hc *HandlerContext) handleMileageExport(c *gin.Context) {
	if hc.mileage == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	var start *time.Time
	var end *time.Time
	if v := c.Query("start_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			start = &t
		} else {
			respondError(c, http.StatusBadRequest, CodeInvalidDate)
			return
		}
	}
	if v := c.Query("end_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end = &t
		} else {
			respondError(c, http.StatusBadRequest, CodeInvalidDate)
			return
		}
	}

	logs, err := hc.mileage.Export(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=mileage.csv")

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"date", "distance_miles", "start_location", "end_location", "purpose", "receipt_id", "user_id"})
	for _, log := range logs {
		_ = w.Write([]string{
			log.Date.Format("2006-01-02"),
			strconv.FormatFloat(log.DistanceMiles, 'f', 3, 64),
			log.StartLocation,
			log.EndLocation,
			log.Purpose,
			log.ReceiptID,
			log.UserID,
		})
	}
	w.Flush()
}

func (hc *HandlerContext) handleMileageSummary(c *gin.Context) {
	if hc.mileage == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")
	if entityID == "" || startStr == "" || endStr == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}

	rows, err := hc.mileage.SummaryByMonth(c.Request.Context(), entityID, start, end, hc.mileageRates)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{
			"month":       row.Month.Format("2006-01"),
			"total_miles": row.TotalMiles,
			"trip_count":  row.TripCount,
		}
		if row.HasRate {
			item["rate_cents_per_mile"] = row.RateCents
			item["reimbursed_cents"] = row.ReimbCents
		} else {
			item["rate_cents_per_mile"] = nil
			item["reimbursed_cents"] = nil
			item["rate_missing"] = true
		}
		resp = append(resp, item)
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id":  entityID,
		"start_date": startStr,
		"end_date":   endStr,
		"rows":       resp,
	})
}

func mileageResponse(log models.MileageLog, suggestionContext string) gin.H {
	resp := gin.H{
		"id":             log.ID,
		"entity_id":      log.EntityID,
		"user_id":        log.UserID,
		"receipt_id":     log.ReceiptID,
		"date":           log.Date.Format("2006-01-02"),
		"distance_miles": log.DistanceMiles,
		"start_location": log.StartLocation,
		"end_location":   log.EndLocation,
		"purpose":        log.Purpose,
		"created_at":     log.CreatedAt.Format(time.RFC3339),
		"updated_at":     log.UpdatedAt.Format(time.RFC3339),
	}
	if strings.TrimSpace(suggestionContext) != "" {
		resp["suggestion_context"] = suggestionContext
		resp["context"] = suggestionContext
	}
	return resp
}
