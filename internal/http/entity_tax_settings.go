package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
)

type entityTaxSettingsResponse struct {
	TaxYear                         int    `json:"tax_year"`
	HomeOfficeSqFt                  *int   `json:"home_office_sqft,omitempty"`
	HomeTotalSqFt                   *int   `json:"home_total_sqft,omitempty"`
	HomeUtilitiesBusinessUsePercent *int   `json:"home_utilities_business_use_percent,omitempty"`
	CellPhoneBusinessUsePercent     *int   `json:"cell_phone_business_use_percent,omitempty"`
	HomeInternetBusinessUsePercent  *int   `json:"home_internet_business_use_percent,omitempty"`
	UpdatedAt                       string `json:"updated_at,omitempty"`
}

type entityTaxSettingsRequest struct {
	TaxYear                        *int `json:"tax_year"`
	HomeOfficeSqFt                 *int `json:"home_office_sqft"`
	HomeTotalSqFt                  *int `json:"home_total_sqft"`
	CellPhoneBusinessUsePercent    *int `json:"cell_phone_business_use_percent"`
	HomeInternetBusinessUsePercent *int `json:"home_internet_business_use_percent"`
}

func (hc *HandlerContext) handleGetEntityTaxSettings(c *gin.Context) {
	if hc.entityTaxSettings == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Param("id")
	taxYear, err := entityTaxYearFromQuery(c.Query("year"))
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	settings, err := hc.entityTaxSettings.Get(c.Request.Context(), entityID, taxYear)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, entityTaxSettingsResponseFromDB(settings))
}

func (hc *HandlerContext) handleUpdateEntityTaxSettings(c *gin.Context) {
	if hc.entityTaxSettings == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Param("id")
	var req entityTaxSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.TaxYear == nil {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	taxYear := *req.TaxYear
	if taxYear < 1900 || taxYear > 9999 {
		respondError(c, http.StatusBadRequest, CodeInvalidTaxYear)
		return
	}
	if err := validateHomeUseSettings(req.HomeOfficeSqFt, req.HomeTotalSqFt, req.CellPhoneBusinessUsePercent, req.HomeInternetBusinessUsePercent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := hc.entityTaxSettings.Upsert(
		c.Request.Context(),
		entityID,
		taxYear,
		nullInt64(req.HomeOfficeSqFt),
		nullInt64(req.HomeTotalSqFt),
		nullInt64(req.CellPhoneBusinessUsePercent),
		nullInt64(req.HomeInternetBusinessUsePercent),
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, entityTaxSettingsResponseFromDB(updated))
}

func entityTaxSettingsResponseFromDB(settings db.EntityTaxSettings) entityTaxSettingsResponse {
	resp := entityTaxSettingsResponse{
		TaxYear: settings.TaxYear,
	}
	if !settings.UpdatedAt.IsZero() {
		resp.UpdatedAt = settings.UpdatedAt.UTC().Format(time.RFC3339)
	}
	resp.HomeOfficeSqFt = intPtr(settings.HomeOfficeSqFt)
	resp.HomeTotalSqFt = intPtr(settings.HomeTotalSqFt)
	resp.CellPhoneBusinessUsePercent = intPtr(settings.CellPhoneBusinessUsePercent)
	resp.HomeInternetBusinessUsePercent = intPtr(settings.HomeInternetBusinessUsePercent)
	if percent, ok := db.UtilitiesBusinessUsePercent(settings.HomeOfficeSqFt, settings.HomeTotalSqFt); ok {
		resp.HomeUtilitiesBusinessUsePercent = &percent
	}
	return resp
}

func entityTaxYearFromQuery(value string) (int, error) {
	if value == "" {
		return time.Now().Year(), nil
	}
	year, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if year < 1900 || year > 9999 {
		return 0, errors.New("invalid year")
	}
	return year, nil
}

func validateHomeUseSettings(homeOfficeSqFt, homeTotalSqFt, cellPhoneBusinessUsePercent, homeInternetBusinessUsePercent *int) error {
	if homeOfficeSqFt != nil && *homeOfficeSqFt < 0 {
		return errors.New("INVALID_HOME_OFFICE_SQFT")
	}
	if homeTotalSqFt != nil && *homeTotalSqFt < 0 {
		return errors.New("INVALID_HOME_TOTAL_SQFT")
	}
	if homeOfficeSqFt != nil && homeTotalSqFt != nil && *homeOfficeSqFt > *homeTotalSqFt {
		return errors.New("INVALID_HOME_OFFICE_RATIO")
	}
	if cellPhoneBusinessUsePercent != nil && (*cellPhoneBusinessUsePercent < 0 || *cellPhoneBusinessUsePercent > 100) {
		return errors.New("INVALID_CELL_PHONE_PERCENT")
	}
	if homeInternetBusinessUsePercent != nil && (*homeInternetBusinessUsePercent < 0 || *homeInternetBusinessUsePercent > 100) {
		return errors.New("INVALID_HOME_INTERNET_PERCENT")
	}
	return nil
}

func nullInt64(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func intPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}
