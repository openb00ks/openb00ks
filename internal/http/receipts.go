package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/queue"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/spectrum-labs-tech/go-toolkit/pkg/upload"
)

type ReceiptHandler struct {
	maxBytes int64
}

func NewReceiptHandler(maxBytes int64) *ReceiptHandler {
	return &ReceiptHandler{maxBytes: maxBytes}
}

func (hc *HandlerContext) handleReceiptUpload(c *gin.Context) {
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

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			_ = c.Error(err)
		}
	}()

	if hc.receiptCfg.maxBytes > 0 && header.Size > hc.receiptCfg.maxBytes {
		respondError(c, http.StatusRequestEntityTooLarge, CodeFileTooLarge)
		return
	}

	if err := upload.Validate(header,
		upload.AllowMIME("image/jpeg", "image/png", "application/pdf"),
	); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidFileType)
		return
	}
	contentType := header.Header.Get("Content-Type")

	name := sanitizeFilename(header.Filename)
	totalCents := int64(0)
	if v := c.PostForm("total_cents"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			totalCents = n
		}
	}

	key, err := hc.objects.Put(c.Request.Context(), name, contentType, header.Size, file)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	receipt, err := hc.receiptStore.Create(c.Request.Context(), entityID, key, contentType, "uploaded", "receipt", header.Filename, header.Size, totalCents)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.auditEvent(c.Request.Context(), receipt.EntityID, userID, "receipt", receipt.ID, "receipt.created", nil, map[string]interface{}{
		"status":        receipt.Status,
		"storage_key":   receipt.StorageKey,
		"content_type":  receipt.ContentType,
		"size_bytes":    receipt.SizeBytes,
		"total_cents":   receipt.TotalCents,
		"original_name": receipt.OriginalName,
	})

	if hc.queue != nil {
		_, _ = hc.queue.Enqueue(c.Request.Context(), queue.EnqueueRequest{ReceiptID: receipt.ID, Stage: queue.StageOCR})
	}

	if hc.receiptMeta != nil {
		if ctxText := formSuggestionContext(c.PostForm("suggestion_context"), c.PostForm("context")); ctxText != "" {
			_ = hc.receiptMeta.UpsertSuggestionContext(c.Request.Context(), receipt.ID, ctxText)
		}
	}

	if hc.tags != nil {
		tags := parseTags(c)
		if len(tags) > 0 {
			receiptTags, err := hc.ensureTags(c.Request.Context(), receipt.EntityID, tags)
			if err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
			if err := hc.tags.SetReceiptTags(c.Request.Context(), receipt.ID, receiptTags); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError)
				return
			}
		}
	}
	hc.indexReceipt(c, receipt)
	hc.metrics.receiptUploaded(c.Request.Context())

	c.JSON(http.StatusCreated, gin.H{
		"id":          receipt.ID,
		"status":      receipt.Status,
		"storage_key": receipt.StorageKey,
	})
}

func (hc *HandlerContext) handleReceiptGet(c *gin.Context) {
	if hc.receiptStore == nil || hc.objects == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	_, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
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
		"total_cents":   receipt.TotalCents,
		"uploaded_at":   receipt.UploadedAt.Format(time.RFC3339),
		"attached_at":   formatTime(receipt.AttachedAt),
		"original_name": receipt.OriginalName,
		"url":           url,
	}
	if receipt.AISummary != nil {
		resp["ai_summary"] = receipt.AISummary
	}
	if receipt.ResolvedVendorID != "" {
		resp["resolved_vendor_id"] = receipt.ResolvedVendorID
	}
	if hc.drafts != nil {
		if draft, err := hc.drafts.GetByReceiptID(c.Request.Context(), receipt.ID); err == nil {
			resp["draft"] = draftResponse(draft)
		}
	}
	if hc.tags != nil {
		if tags, err := hc.tags.ListByReceiptID(c.Request.Context(), receipt.ID); err == nil {
			resp["tags"] = tags
		}
	}
	if hc.receiptMeta != nil {
		if ctxText, err := hc.receiptMeta.GetSuggestionContext(c.Request.Context(), receipt.ID); err == nil && ctxText != "" {
			resp["suggestion_context"] = ctxText
			resp["context"] = ctxText
		}
	}
	if hc.errors != nil {
		if errs, err := hc.errors.ListByReceiptID(c.Request.Context(), receipt.ID, 50); err == nil {
			resp["errors"] = errs
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (hc *HandlerContext) handleReceiptStatus(c *gin.Context) {
	if hc.receiptStore == nil || hc.receiptJobs == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	receipt, err := hc.receiptStore.GetByID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	jobs, err := hc.receiptJobs.ListByReceiptID(c.Request.Context(), receiptID, 20)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	jobRows := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		jobRows = append(jobRows, receiptJobResponse(job))
	}
	var latest gin.H
	if len(jobRows) > 0 {
		latest = jobRows[0]
	}

	errorsList := []models.ProcessingError{}
	if hc.errors != nil {
		if rows, err := hc.errors.ListByReceiptID(c.Request.Context(), receiptID, 10); err == nil {
			errorsList = rows
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"receipt_id": receipt.ID,
		"entity_id":  receipt.EntityID,
		"status":     receipt.Status,
		"latest_job": latest,
		"jobs":       jobRows,
		"errors":     errorsList,
	})
}

type updateReceiptTagsRequest struct {
	Tags []string `json:"tags"`
}

func (hc *HandlerContext) handleReceiptTagsUpdate(c *gin.Context) {
	if hc.receiptStore == nil || hc.tags == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	entityID, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	var req updateReceiptTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	receiptTags, err := hc.ensureTags(c.Request.Context(), entityID, req.Tags)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if err := hc.tags.SetReceiptTags(c.Request.Context(), receiptID, receiptTags); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if receipt, err := hc.receiptStore.GetByID(c.Request.Context(), receiptID); err == nil {
		hc.indexReceipt(c, receipt)
	}
	c.JSON(http.StatusOK, gin.H{"tags": receiptTags})
}

func (hc *HandlerContext) handleReceiptList(c *gin.Context) {
	if hc.receiptStore == nil {
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
	receipts, err := hc.receiptStore.List(c.Request.Context(), entityID, status, limit)
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
			"total_cents":   receipt.TotalCents,
			"uploaded_at":   receipt.UploadedAt.Format(time.RFC3339),
			"attached_at":   formatTime(receipt.AttachedAt),
			"original_name": receipt.OriginalName,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"rows":      resp,
	})
}

func (hc *HandlerContext) handleReceiptRequeue(c *gin.Context) {
	if hc.receiptStore == nil || hc.queue == nil || hc.receiptJobs == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	_, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	jobID, err := hc.receiptJobs.GetIDByReceiptID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if err := hc.queue.Requeue(c.Request.Context(), jobID); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"job_id": jobID,
		"status": "queued",
	})
}

func (hc *HandlerContext) handleReceiptOCR(c *gin.Context) {
	if hc.receiptStore == nil || hc.receiptOCR == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	_, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	history, err := hc.receiptOCR.ListByReceiptID(c.Request.Context(), receiptID, 20)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": history})
}

func (hc *HandlerContext) handleReceiptOCRRerun(c *gin.Context) {
	hc.rerunStage(c, queue.StageOCR)
}

func (hc *HandlerContext) handleReceiptSuggestion(c *gin.Context) {
	if hc.receiptStore == nil || hc.suggestions == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	_, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	history, err := hc.suggestions.ListByReceiptID(c.Request.Context(), receiptID, 20)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	rows := make([]gin.H, 0, len(history))
	var sourceURL string
	if hc.objects != nil {
		if receipt, err := hc.receiptStore.GetByID(c.Request.Context(), receiptID); err == nil {
			if url, err := hc.objects.GetURL(c.Request.Context(), receipt.StorageKey); err == nil {
				sourceURL = url
			}
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

func (hc *HandlerContext) handleReceiptSuggestionRerun(c *gin.Context) {
	hc.rerunStage(c, queue.StageSuggest)
}

func (hc *HandlerContext) handleReceiptDraftRerun(c *gin.Context) {
	hc.rerunStage(c, queue.StageDraft)
}

type receiptSuggestionsBatchRequest struct {
	ReceiptIDs []string `json:"receipt_ids"`
}

func (hc *HandlerContext) handleReceiptSuggestionsBatch(c *gin.Context) {
	if hc.receiptStore == nil || hc.suggestions == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req receiptSuggestionsBatchRequest
	// The role-resolver middleware (receiptIDsFromBody) already read the body via ShouldBindBodyWith,
	// which consumes + caches it — so re-read from that cache here. A plain ShouldBindJSON would hit the
	// now-drained request stream (EOF) and 400 every batch call.
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if len(req.ReceiptIDs) == 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}

	rows := make([]gin.H, 0, len(req.ReceiptIDs))
	allowedReceipts, ok := EntityIDs(c)
	allowedSet := map[string]struct{}{}
	if ok {
		for _, receiptID := range allowedReceipts {
			allowedSet[receiptID] = struct{}{}
		}
	}
	for _, receiptID := range req.ReceiptIDs {
		if ok {
			if _, allowed := allowedSet[receiptID]; !allowed {
				continue
			}
		}
		suggestion, err := hc.suggestions.LatestByReceiptID(c.Request.Context(), receiptID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				continue
			}
			respondError(c, http.StatusInternalServerError, CodeInternalError)
			return
		}
		rows = append(rows, gin.H{
			"receipt_id":    receiptID,
			"suggestion_id": suggestion.ID,
			"status":        suggestion.Status,
			"confidence":    suggestion.Confidence,
			"cost_cents":    hc.projectedCostCents(suggestion),
			"total_tokens":  suggestion.TotalTokens,
		})
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

func (hc *HandlerContext) projectedCostCents(suggestion models.ReceiptSuggestion) int64 {
	if suggestion.CostCents > 0 {
		return suggestion.CostCents
	}
	if suggestion.PromptTokens == 0 && suggestion.CompletionTokens == 0 {
		return 0
	}
	return suggest.EstimateCostCents(suggestion.PromptTokens, suggestion.CompletionTokens, hc.aiPricing)
}

func (hc *HandlerContext) rerunStage(c *gin.Context, stage queue.JobStage) {
	if hc.receiptStore == nil || hc.queue == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	receiptID := c.Param("id")
	entityID, err := hc.receiptStore.GetEntityID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if err := hc.receiptStore.UpdateStatus(c.Request.Context(), receiptID, "queued"); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	job, err := hc.queue.Enqueue(c.Request.Context(), queue.EnqueueRequest{ReceiptID: receiptID, Stage: stage})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.auditEvent(c.Request.Context(), entityID, userIDFromContext(c), "receipt", receiptID, "receipt.rerun", nil, map[string]interface{}{
		"stage":  stage,
		"job_id": job.ID,
	})
	c.JSON(http.StatusOK, gin.H{
		"job_id": job.ID,
		"stage":  job.Stage,
		"status": "queued",
	})
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func receiptJobResponse(job models.ReceiptJob) gin.H {
	return gin.H{
		"id":           job.ID,
		"receipt_id":   job.ReceiptID,
		"stage":        job.Stage,
		"status":       job.Status,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
		"locked_until": formatTime(job.LockedUntil),
		"locked_by":    job.LockedBy,
		"last_error":   job.LastError,
		"created_at":   job.CreatedAt.Format(time.RFC3339),
		"updated_at":   job.UpdatedAt.Format(time.RFC3339),
	}
}

func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, " ", "_")
	return base
}

func parseTags(c *gin.Context) []string {
	tags := make([]string, 0)
	if values := c.PostFormArray("tags"); len(values) > 0 {
		tags = append(tags, values...)
	}
	if values := c.PostFormArray("tags[]"); len(values) > 0 {
		tags = append(tags, values...)
	}
	if v := c.PostForm("tags"); v != "" {
		tags = append(tags, strings.Split(v, ",")...)
	}
	return tags
}

func (hc *HandlerContext) ensureTags(ctx context.Context, entityID string, raw []string) ([]models.Tag, error) {
	tags := make([]models.Tag, 0, len(raw))
	seen := make(map[string]struct{})
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tag, err := hc.tags.Ensure(ctx, entityID, name)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}
