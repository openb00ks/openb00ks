package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/queue"
)

func (hc *HandlerContext) handleImportUpload(c *gin.Context) {
	if hc.receiptCfg == nil || hc.receiptStore == nil || hc.objects == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)
	entityID := c.PostForm("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	var (
		reader      io.Reader
		origName    string
		contentType string
		sizeBytes   int64
		importText  string
	)

	if file, header, err := c.Request.FormFile("file"); err == nil {
		defer func() {
			if err := file.Close(); err != nil {
				_ = c.Error(err)
			}
		}()
		if hc.receiptCfg.maxBytes > 0 && header.Size > hc.receiptCfg.maxBytes {
			respondError(c, http.StatusRequestEntityTooLarge, CodeFileTooLarge)
			return
		}
		contentType = header.Header.Get("Content-Type")
		origName = header.Filename
		if contentType == "" && strings.HasSuffix(strings.ToLower(origName), ".csv") {
			contentType = "text/csv"
		}
		sizeBytes = header.Size
		if contentType == "text/csv" || contentType == "text/plain" {
			body, err := io.ReadAll(file)
			if err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
			sizeBytes = int64(len(body))
			importText = string(body)
			reader = bytes.NewReader(body)
		} else {
			reader = file
		}
	} else {
		text := strings.TrimSpace(c.PostForm("text"))
		if text == "" {
			respondError(c, http.StatusBadRequest, CodeMissingFields)
			return
		}
		contentType = c.PostForm("content_type")
		if contentType == "" {
			contentType = "text/csv"
		}
		origName = c.PostForm("filename")
		if origName == "" {
			origName = "import.csv"
		}
		reader = bytes.NewBufferString(text)
		sizeBytes = int64(len(text))
		importText = text
		if hc.receiptCfg.maxBytes > 0 && sizeBytes > hc.receiptCfg.maxBytes {
			respondError(c, http.StatusRequestEntityTooLarge, CodeFileTooLarge)
			return
		}
	}

	if !isAllowedImportType(contentType) {
		respondError(c, http.StatusBadRequest, CodeInvalidFileType)
		return
	}

	name := sanitizeFilename(origName)
	key, err := hc.objects.Put(c.Request.Context(), name, contentType, sizeBytes, reader)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	receipt, err := hc.receiptStore.Create(c.Request.Context(), entityID, key, contentType, "uploaded", "import", origName, sizeBytes, 0)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.auditEvent(c.Request.Context(), receipt.EntityID, userID, "import", receipt.ID, "import.created", nil, map[string]interface{}{
		"status":        receipt.Status,
		"storage_key":   receipt.StorageKey,
		"content_type":  receipt.ContentType,
		"size_bytes":    receipt.SizeBytes,
		"original_name": receipt.OriginalName,
	})

	stage := queue.StageOCR
	if contentType == "text/csv" || contentType == "text/plain" {
		stage = queue.StageSuggest
	}
	if hc.queue != nil {
		_, _ = hc.queue.Enqueue(c.Request.Context(), queue.EnqueueRequest{ReceiptID: receipt.ID, Stage: stage})
	}
	hc.indexReceipt(c, receipt)

	if importText != "" && hc.receiptOCR != nil {
		runVersion := 1
		if latest, err := hc.receiptOCR.LatestByReceiptID(c.Request.Context(), receipt.ID); err == nil {
			runVersion = latest.RunVersion + 1
		}
		_, _ = hc.receiptOCR.Create(c.Request.Context(), models.ReceiptOCR{
			ReceiptID:  receipt.ID,
			Provider:   "import",
			Status:     "provided",
			RawText:    importText,
			DataJSON:   []byte(`{}`),
			RunVersion: runVersion,
		})
	}

	if hc.receiptMeta != nil {
		if ctxText := formSuggestionContext(c.PostForm("suggestion_context"), c.PostForm("context")); ctxText != "" {
			_ = hc.receiptMeta.UpsertSuggestionContext(c.Request.Context(), receipt.ID, ctxText)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            receipt.ID,
		"entity_id":     receipt.EntityID,
		"status":        receipt.Status,
		"kind":          receipt.Kind,
		"content_type":  receipt.ContentType,
		"size_bytes":    receipt.SizeBytes,
		"uploaded_at":   receipt.UploadedAt.Format(time.RFC3339),
		"original_name": receipt.OriginalName,
	})
}

func (hc *HandlerContext) handleImportList(c *gin.Context) {
	if hc.receiptStore == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	limit := queryLimit(c, 100, 1000)
	status := c.Query("status")
	receipts, err := hc.receiptStore.ListByKind(c.Request.Context(), entityID, "import", status, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(receipts))
	for _, receipt := range receipts {
		resp = append(resp, gin.H{
			"id":            receipt.ID,
			"entity_id":     receipt.EntityID,
			"status":        receipt.Status,
			"kind":          receipt.Kind,
			"content_type":  receipt.ContentType,
			"size_bytes":    receipt.SizeBytes,
			"uploaded_at":   receipt.UploadedAt.Format(time.RFC3339),
			"original_name": receipt.OriginalName,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"rows":      resp,
	})
}

func (hc *HandlerContext) handleImportGet(c *gin.Context) {
	if hc.receiptStore == nil || hc.objects == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), importID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if receipt.Kind != "import" {
		respondError(c, http.StatusNotFound, CodeNotFound)
		return
	}
	url, err := hc.objects.GetURL(c.Request.Context(), receipt.StorageKey)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := gin.H{
		"id":            receipt.ID,
		"entity_id":     receipt.EntityID,
		"status":        receipt.Status,
		"kind":          receipt.Kind,
		"content_type":  receipt.ContentType,
		"size_bytes":    receipt.SizeBytes,
		"uploaded_at":   receipt.UploadedAt.Format(time.RFC3339),
		"original_name": receipt.OriginalName,
		"url":           url,
	}
	if hc.receiptMeta != nil {
		if ctxText, err := hc.receiptMeta.GetSuggestionContext(c.Request.Context(), receipt.ID); err == nil && ctxText != "" {
			resp["suggestion_context"] = ctxText
			resp["context"] = ctxText
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleImportSuggestion(c *gin.Context) {
	if hc.receiptStore == nil || hc.suggestions == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), importID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if receipt.Kind != "import" {
		respondError(c, http.StatusNotFound, CodeNotFound)
		return
	}
	history, err := hc.suggestions.ListByReceiptID(c.Request.Context(), importID, 20)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	rows := make([]gin.H, 0, len(history))
	sourceURL := ""
	if hc.objects != nil {
		if url, err := hc.objects.GetURL(c.Request.Context(), receipt.StorageKey); err == nil {
			sourceURL = url
		}
	}
	for _, suggestion := range history {
		costCents := hc.projectedCostCents(suggestion)
		row := gin.H{
			"id":                suggestion.ID,
			"receipt_id":        suggestion.ReceiptID,
			"provider":          suggestion.Provider,
			"model":             suggestion.Model,
			"status":            suggestion.Status,
			"prompt_json":       suggestion.PromptJSON,
			"raw_response":      suggestion.RawJSON,
			"parsed_json":       suggestion.ParsedJSON,
			"confidence":        suggestion.Confidence,
			"error":             suggestion.Error,
			"input_hash":        suggestion.InputHash,
			"run_version":       suggestion.RunVersion,
			"created_at":        suggestion.CreatedAt.Format(time.RFC3339),
			"prompt_tokens":     suggestion.PromptTokens,
			"completion_tokens": suggestion.CompletionTokens,
			"total_tokens":      suggestion.TotalTokens,
			"cost_cents":        costCents,
		}
		if sourceURL != "" {
			row["source_url"] = sourceURL
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

func (hc *HandlerContext) handleImportSuggestionRerun(c *gin.Context) {
	hc.rerunStage(c, queue.StageSuggest)
}

func (hc *HandlerContext) handleImportRowsList(c *gin.Context) {
	if hc.importRows == nil || hc.receiptStore == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), importID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if receipt.Kind != "import" {
		respondError(c, http.StatusNotFound, CodeNotFound)
		return
	}
	rows, err := hc.importRows.ListByReceiptID(c.Request.Context(), importID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, importRowResponse(row))
	}
	c.JSON(http.StatusOK, gin.H{"rows": resp})
}

type updateImportRowRequest struct {
	AccountID string `json:"account_id"`
}

func (hc *HandlerContext) handleImportRowUpdate(c *gin.Context) {
	if hc.importRows == nil || hc.receiptStore == nil || hc.accounts == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	rowIndex, ok := parseRowIndexParam(c)
	if !ok {
		return
	}
	var req updateImportRowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if strings.TrimSpace(req.AccountID) == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), importID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	accountEntityID, err := hc.accounts.GetEntityID(c.Request.Context(), req.AccountID)
	if err != nil || accountEntityID != receipt.EntityID {
		respondError(c, http.StatusBadRequest, CodeInvalidAccount)
		return
	}
	row, err := hc.importRows.UpdateAccount(c.Request.Context(), importID, rowIndex, req.AccountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, importRowResponse(row))
}

func (hc *HandlerContext) handleImportRowPost(c *gin.Context) {
	if hc.importRows == nil || hc.transactions == nil || hc.accounts == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	rowIndex, ok := parseRowIndexParam(c)
	if !ok {
		return
	}
	row, err := hc.importRows.GetByReceiptAndIndex(c.Request.Context(), importID, rowIndex)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if row.TransactionID != "" || row.Status == "posted" {
		respondError(c, http.StatusConflict, CodeImportRowAlreadyPosted)
		return
	}
	if row.AccountID == "" {
		respondError(c, http.StatusBadRequest, CodeImportRowAccountRequired)
		return
	}
	offsetAccountID, err := hc.importOffsetAccountID(c, row)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeDefaultCashAccountRequired)
		return
	}
	tr, err := hc.postImportRow(c, row, offsetAccountID)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidTransaction)
		return
	}
	row, err = hc.importRows.GetByReceiptAndIndex(c.Request.Context(), importID, rowIndex)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.auditEvent(c.Request.Context(), row.EntityID, userIDFromContext(c), "import_row", row.ID, "import_row.posted", nil, map[string]interface{}{
		"transaction_id": tr.ID,
		"receipt_id":     row.ReceiptID,
		"row_index":      row.RowIndex,
	})
	c.JSON(http.StatusCreated, gin.H{
		"transaction": transactionResponse(tr),
		"row":         importRowResponse(row),
	})
}

func (hc *HandlerContext) handleImportRowsPostMapped(c *gin.Context) {
	if hc.importRows == nil || hc.receiptStore == nil || hc.transactions == nil || hc.accounts == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	importID := c.Param("id")
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), importID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if receipt.Kind != "import" {
		respondError(c, http.StatusNotFound, CodeNotFound)
		return
	}
	offsetAccountID, err := hc.importReceiptOffsetAccountID(c, receipt.ID, receipt.EntityID)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeDefaultCashAccountRequired)
		return
	}
	rows, err := hc.importRows.ListByReceiptID(c.Request.Context(), importID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	results := make([]gin.H, 0, len(rows))
	posted := 0
	skipped := 0
	failed := 0
	for _, row := range rows {
		result := gin.H{"row_index": row.RowIndex, "status": row.Status}
		switch {
		case row.TransactionID != "" || row.Status == "posted":
			result["result"] = "skipped"
			result["reason"] = "already posted"
			skipped++
		case row.AccountID == "":
			result["result"] = "skipped"
			result["reason"] = "missing account"
			skipped++
		default:
			tr, err := hc.postImportRow(c, row, offsetAccountID)
			if err != nil {
				result["result"] = "failed"
				result["reason"] = err.Error()
				failed++
			} else {
				result["result"] = "posted"
				result["transaction_id"] = tr.ID
				posted++
			}
		}
		results = append(results, result)
	}
	if posted > 0 {
		hc.auditEvent(c.Request.Context(), receipt.EntityID, userIDFromContext(c), "import", receipt.ID, "import_rows.bulk_posted", nil, map[string]interface{}{
			"posted":  posted,
			"skipped": skipped,
			"failed":  failed,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"posted":  posted,
		"skipped": skipped,
		"failed":  failed,
		"rows":    results,
	})
}

func (hc *HandlerContext) postImportRow(c *gin.Context, row models.ImportRow, cashAccountID string) (models.Transaction, error) {
	entries := importRowEntries(row, cashAccountID)
	if len(entries) == 0 {
		return models.Transaction{}, errors.New("invalid import row")
	}
	tr, outEntries, err := hc.transactions.Create(c.Request.Context(), row.EntityID, row.Date, importRowMemo(row), "", entries)
	if err != nil {
		return models.Transaction{}, err
	}
	if err := hc.importRows.MarkPosted(c.Request.Context(), row.ID, tr.ID); err != nil {
		return models.Transaction{}, err
	}
	hc.indexTransaction(c, tr, outEntries)
	return tr, nil
}

func (hc *HandlerContext) importOffsetAccountID(c *gin.Context, row models.ImportRow) (string, error) {
	return hc.importReceiptOffsetAccountID(c, row.ReceiptID, row.EntityID)
}

func (hc *HandlerContext) importReceiptOffsetAccountID(c *gin.Context, receiptID, entityID string) (string, error) {
	if hc.accountStatements != nil {
		if statement, err := hc.accountStatements.GetBySourceReceiptID(c.Request.Context(), receiptID); err == nil && statement.EntityID == entityID && statement.AccountID != "" {
			return statement.AccountID, nil
		} else if err != nil && !errors.Is(err, db.ErrNotFound) {
			return "", err
		}
	}
	cash, err := hc.accounts.FindDefaultCashAccount(c.Request.Context(), entityID)
	if err != nil {
		return "", err
	}
	return cash.ID, nil
}

func parseRowIndexParam(c *gin.Context) (int, bool) {
	rowIndex, err := strconv.Atoi(c.Param("row_index"))
	if err != nil || rowIndex <= 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRowIndex)
		return 0, false
	}
	return rowIndex, true
}

func importRowEntries(row models.ImportRow, cashAccountID string) []models.DraftEntry {
	if row.AmountCents <= 0 || row.AccountID == "" || cashAccountID == "" {
		return nil
	}
	if row.Direction == "inflow" {
		return []models.DraftEntry{
			{AccountID: cashAccountID, DebitCents: row.AmountCents},
			{AccountID: row.AccountID, CreditCents: row.AmountCents},
		}
	}
	return []models.DraftEntry{
		{AccountID: row.AccountID, DebitCents: row.AmountCents},
		{AccountID: cashAccountID, CreditCents: row.AmountCents},
	}
}

func importRowMemo(row models.ImportRow) string {
	if row.Memo != "" && row.Memo != row.Vendor {
		return row.Vendor + " - " + row.Memo
	}
	return row.Vendor
}

func importRowResponse(row models.ImportRow) gin.H {
	return gin.H{
		"id":             row.ID,
		"receipt_id":     row.ReceiptID,
		"entity_id":      row.EntityID,
		"row_index":      row.RowIndex,
		"date":           row.Date.Format("2006-01-02"),
		"vendor":         row.Vendor,
		"memo":           row.Memo,
		"amount_cents":   row.AmountCents,
		"direction":      row.Direction,
		"account_id":     row.AccountID,
		"fingerprint":    row.Fingerprint,
		"status":         row.Status,
		"transaction_id": row.TransactionID,
		"raw_json":       row.RawJSON,
		"created_at":     row.CreatedAt.Format(time.RFC3339),
		"updated_at":     row.UpdatedAt.Format(time.RFC3339),
	}
}

func isAllowedImportType(contentType string) bool {
	switch strings.ToLower(contentType) {
	case "image/jpeg", "image/png", "application/pdf", "text/csv", "text/plain":
		return true
	default:
		return false
	}
}
