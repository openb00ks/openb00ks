package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
)

func (hc *HandlerContext) handleMileageRatesList(c *gin.Context) {
	if hc.mileageRates == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	rates, err := hc.mileageRates.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rates))
	for _, rate := range rates {
		resp = append(resp, gin.H{
			"year":                rate.Year,
			"rate_cents_per_mile": rate.RateCentsPerMile,
			"created_at":          rate.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at":          rate.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"rows": resp})
}

type upsertRateRequest struct {
	Year             int `json:"year"`
	RateCentsPerMile int `json:"rate_cents_per_mile"`
}

func (hc *HandlerContext) handleMileageRatesUpsert(c *gin.Context) {
	if hc.mileageRates == nil || hc.admin == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req upsertRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Year == 0 {
		yearStr := c.Param("year")
		year, err := strconv.Atoi(yearStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidYear)
			return
		}
		req.Year = year
	}
	if req.Year <= 0 || req.RateCentsPerMile <= 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	if req.Year < 2000 || req.Year > 2100 {
		respondError(c, http.StatusBadRequest, CodeInvalidYear)
		return
	}
	rate, err := hc.mileageRates.Upsert(c.Request.Context(), req.Year, req.RateCentsPerMile)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"year":                rate.Year,
		"rate_cents_per_mile": rate.RateCentsPerMile,
		"created_at":          rate.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":          rate.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (hc *HandlerContext) handleMileageRatesGet(c *gin.Context) {
	if hc.mileageRates == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	yearStr := c.Param("year")
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidYear)
		return
	}
	rate, err := hc.mileageRates.Get(c.Request.Context(), year)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"year":                rate.Year,
		"rate_cents_per_mile": rate.RateCentsPerMile,
		"created_at":          rate.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":          rate.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
