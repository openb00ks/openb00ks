package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type accountStatementRequest struct {
	EntityID             string `json:"entity_id"`
	AccountID            string `json:"account_id"`
	SourceReceiptID      string `json:"source_receipt_id"`
	PeriodStart          string `json:"period_start"`
	PeriodEnd            string `json:"period_end"`
	StartingBalanceCents int64  `json:"starting_balance_cents"`
	EndingBalanceCents   int64  `json:"ending_balance_cents"`
	Status               string `json:"status"`
	Notes                string `json:"notes"`
}

type accountStatementPatchRequest struct {
	AccountID            *string `json:"account_id"`
	SourceReceiptID      *string `json:"source_receipt_id"`
	PeriodStart          *string `json:"period_start"`
	PeriodEnd            *string `json:"period_end"`
	StartingBalanceCents *int64  `json:"starting_balance_cents"`
	EndingBalanceCents   *int64  `json:"ending_balance_cents"`
	Status               *string `json:"status"`
	Notes                *string `json:"notes"`
}

func (hc *HandlerContext) handleAccountStatementsList(c *gin.Context) {
	if hc.accountStatements == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	accountID := c.Query("account_id")
	start, ok := optionalDateQuery(c, "start_date")
	if !ok {
		return
	}
	end, ok := optionalDateQuery(c, "end_date")
	if !ok {
		return
	}
	limit := queryLimit(c, 200, 1000)
	rows, err := hc.accountStatements.List(c.Request.Context(), entityID, accountID, start, end, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, accountStatementResponse(row))
	}
	c.JSON(http.StatusOK, gin.H{"rows": resp})
}

func (hc *HandlerContext) handleAccountStatementCreate(c *gin.Context) {
	if hc.accountStatements == nil || hc.accounts == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req accountStatementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || req.AccountID == "" || req.PeriodStart == "" || req.PeriodEnd == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	start, end, ok := parseStatementPeriod(c, req.PeriodStart, req.PeriodEnd)
	if !ok {
		return
	}
	if !hc.validateStatementAccount(c, req.EntityID, req.AccountID) {
		return
	}
	if !hc.validateStatementReceipt(c, req.EntityID, req.SourceReceiptID) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "draft"
	}
	if !validAccountStatementStatus(status) {
		respondError(c, http.StatusBadRequest, CodeInvalidStatus)
		return
	}
	statement, err := hc.accountStatements.Create(c.Request.Context(), models.AccountStatement{
		EntityID:             req.EntityID,
		AccountID:            req.AccountID,
		SourceReceiptID:      req.SourceReceiptID,
		PeriodStart:          start,
		PeriodEnd:            end,
		StartingBalanceCents: req.StartingBalanceCents,
		EndingBalanceCents:   req.EndingBalanceCents,
		Status:               status,
		Notes:                strings.TrimSpace(req.Notes),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexAccountStatement(c, statement)
	c.JSON(http.StatusCreated, accountStatementResponse(statement))
}

func (hc *HandlerContext) handleAccountStatementUpdate(c *gin.Context) {
	if hc.accountStatements == nil || hc.accounts == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	statementID := c.Param("id")
	current, err := hc.accountStatements.GetByID(c.Request.Context(), statementID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	var req accountStatementPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	patch := db.AccountStatementPatch{
		AccountID:            trimStringPtr(req.AccountID),
		SourceReceiptID:      trimStringPtr(req.SourceReceiptID),
		StartingBalanceCents: req.StartingBalanceCents,
		EndingBalanceCents:   req.EndingBalanceCents,
		Status:               trimStringPtr(req.Status),
		Notes:                trimStringPtr(req.Notes),
	}
	if patch.AccountID != nil && !hc.validateStatementAccount(c, current.EntityID, *patch.AccountID) {
		return
	}
	if patch.SourceReceiptID != nil && !hc.validateStatementReceipt(c, current.EntityID, *patch.SourceReceiptID) {
		return
	}
	if patch.Status != nil && !validAccountStatementStatus(*patch.Status) {
		respondError(c, http.StatusBadRequest, CodeInvalidStatus)
		return
	}
	if req.PeriodStart != nil {
		start, ok := parseDateValue(c, *req.PeriodStart)
		if !ok {
			return
		}
		patch.PeriodStart = &start
	}
	if req.PeriodEnd != nil {
		end, ok := parseDateValue(c, *req.PeriodEnd)
		if !ok {
			return
		}
		patch.PeriodEnd = &end
	}
	nextStart := current.PeriodStart
	nextEnd := current.PeriodEnd
	if patch.PeriodStart != nil {
		nextStart = *patch.PeriodStart
	}
	if patch.PeriodEnd != nil {
		nextEnd = *patch.PeriodEnd
	}
	if nextEnd.Before(nextStart) {
		respondError(c, http.StatusBadRequest, CodeInvalidPeriod)
		return
	}
	statement, err := hc.accountStatements.Update(c.Request.Context(), statementID, patch)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexAccountStatement(c, statement)
	c.JSON(http.StatusOK, accountStatementResponse(statement))
}

func (hc *HandlerContext) handleAccountStatementReconcile(c *gin.Context) {
	if hc.accountStatements == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	statement, err := hc.accountStatements.Reconcile(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexAccountStatement(c, statement)
	c.JSON(http.StatusOK, accountStatementResponse(statement))
}

func (hc *HandlerContext) validateStatementAccount(c *gin.Context, entityID, accountID string) bool {
	accountEntityID, err := hc.accounts.GetEntityID(c.Request.Context(), accountID)
	if err != nil || accountEntityID != entityID {
		respondError(c, http.StatusBadRequest, CodeInvalidAccount)
		return false
	}
	return true
}

func (hc *HandlerContext) validateStatementReceipt(c *gin.Context, entityID, receiptID string) bool {
	if receiptID == "" {
		return true
	}
	if hc.receiptStore == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return false
	}
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), receiptID)
	if err != nil || receipt.EntityID != entityID || receipt.Kind != "import" {
		respondError(c, http.StatusBadRequest, CodeInvalidSourceImport)
		return false
	}
	return true
}

func parseStatementPeriod(c *gin.Context, startValue, endValue string) (time.Time, time.Time, bool) {
	start, ok := parseDateValue(c, startValue)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, ok := parseDateValue(c, endValue)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	if end.Before(start) {
		respondError(c, http.StatusBadRequest, CodeInvalidPeriod)
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func parseDateValue(c *gin.Context, value string) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return time.Time{}, false
	}
	return parsed, true
}

func optionalDateQuery(c *gin.Context, name string) (*time.Time, bool) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return nil, true
	}
	parsed, ok := parseDateValue(c, value)
	if !ok {
		return nil, false
	}
	return &parsed, true
}

func validAccountStatementStatus(status string) bool {
	switch status {
	case "draft", "needs_review", "reconciled", "locked":
		return true
	default:
		return false
	}
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func accountStatementResponse(statement models.AccountStatement) gin.H {
	return gin.H{
		"id":                            statement.ID,
		"entity_id":                     statement.EntityID,
		"account_id":                    statement.AccountID,
		"account_name":                  statement.AccountName,
		"account_type":                  statement.AccountType,
		"source_receipt_id":             statement.SourceReceiptID,
		"source_receipt_name":           statement.SourceReceiptName,
		"period_start":                  statement.PeriodStart.Format("2006-01-02"),
		"period_end":                    statement.PeriodEnd.Format("2006-01-02"),
		"starting_balance_cents":        statement.StartingBalanceCents,
		"ending_balance_cents":          statement.EndingBalanceCents,
		"imported_inflow_cents":         statement.ImportedInflowCents,
		"imported_outflow_cents":        statement.ImportedOutflowCents,
		"posted_inflow_cents":           statement.PostedInflowCents,
		"posted_outflow_cents":          statement.PostedOutflowCents,
		"expected_ending_balance_cents": statement.ExpectedEndingBalanceCents,
		"difference_cents":              statement.DifferenceCents,
		"unposted_rows":                 statement.UnpostedRows,
		"status":                        statement.Status,
		"notes":                         statement.Notes,
		"created_at":                    statement.CreatedAt.Format(time.RFC3339),
		"updated_at":                    statement.UpdatedAt.Format(time.RFC3339),
	}
}
