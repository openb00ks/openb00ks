package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

func (hc *HandlerContext) handleSearch(c *gin.Context) {
	if hc.search == nil {
		c.JSON(http.StatusOK, gin.H{"rows": []gin.H{}})
		return
	}
	entityID := c.Query("entity_id")
	query := strings.TrimSpace(c.Query("q"))
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	limit := queryLimit(c, 20, 50)
	kinds := []string{}
	if rawKinds := strings.TrimSpace(c.Query("kinds")); rawKinds != "" {
		kinds = splitSearchValues(rawKinds)
	}
	matches, err := hc.search.SearchDocuments(c.Request.Context(), searchpkg.DocumentQuery{
		TenantID:   tenantIDFromContext(c),
		EntityID:   entityID,
		Query:      query,
		Kinds:      kinds,
		Statuses:   splitSearchValues(c.Query("statuses")),
		AccountIDs: splitSearchValues(c.Query("account_ids")),
		Tags:       splitSearchValues(c.Query("tags")),
		StartDate:  strings.TrimSpace(c.Query("start_date")),
		EndDate:    strings.TrimSpace(c.Query("end_date")),
		Limit:      limit,
	})
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeSearchUnavailable)
		return
	}
	rows := make([]gin.H, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, searchDocumentResponse(match))
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"query":     query,
		"rows":      rows,
	})
}

func (hc *HandlerContext) handleSearchReindex(c *gin.Context) {
	if hc.search == nil || hc.searchSource == nil {
		respondError(c, http.StatusNotImplemented, CodeSearchNotConfigured)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	result, err := (searchpkg.Reindexer{
		Provider: hc.search,
		Source:   hc.searchSource,
	}).ReindexDocuments(c.Request.Context(), searchpkg.ReindexOptions{
		TenantID: tenantIDFromContext(c),
		EntityID: entityID,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (hc *HandlerContext) indexReceipt(c *gin.Context, receipt models.Receipt) {
	if hc.search == nil {
		return
	}
	tagNames := []string{}
	if hc.tags != nil {
		if tags, err := hc.tags.ListByReceiptID(c.Request.Context(), receipt.ID); err == nil {
			tagNames = make([]string, 0, len(tags))
			for _, tag := range tags {
				tagNames = append(tagNames, tag.Name)
			}
		}
	}
	doc := searchpkg.SearchDocumentFromReceipt(tenantIDFromContext(c), searchpkg.ReceiptData{
		ID:           receipt.ID,
		EntityID:     receipt.EntityID,
		Kind:         receipt.Kind,
		Status:       receipt.Status,
		ContentType:  receipt.ContentType,
		OriginalName: receipt.OriginalName,
		TotalCents:   receipt.TotalCents,
		TagNames:     tagNames,
		UploadedAt:   receipt.UploadedAt,
	})
	if err := hc.search.UpsertDocument(c.Request.Context(), doc); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("receipt search index failed", "receipt_id", receipt.ID, "err", err)
	}
}

func (hc *HandlerContext) indexAccount(c *gin.Context, account models.Account) {
	if hc.search == nil {
		return
	}
	doc := searchpkg.SearchDocumentFromAccount(tenantIDFromContext(c), searchpkg.AccountData{
		ID:        account.ID,
		EntityID:  account.EntityID,
		Name:      account.Name,
		Type:      account.Type,
		RoleTags:  account.RoleTags,
		CreatedAt: account.CreatedAt,
	})
	if err := hc.search.UpsertDocument(c.Request.Context(), doc); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("account search index failed", "account_id", account.ID, "err", err)
	}
}

func (hc *HandlerContext) indexAccountStatement(c *gin.Context, statement models.AccountStatement) {
	if hc.search == nil {
		return
	}
	doc := searchpkg.SearchDocumentFromStatement(tenantIDFromContext(c), searchpkg.StatementData{
		ID:                 statement.ID,
		EntityID:           statement.EntityID,
		AccountID:          statement.AccountID,
		AccountName:        statement.AccountName,
		AccountType:        statement.AccountType,
		SourceReceiptName:  statement.SourceReceiptName,
		PeriodStart:        statement.PeriodStart,
		PeriodEnd:          statement.PeriodEnd,
		EndingBalanceCents: statement.EndingBalanceCents,
		Status:             statement.Status,
		Notes:              statement.Notes,
		CreatedAt:          statement.CreatedAt,
		UpdatedAt:          statement.UpdatedAt,
	})
	if err := hc.search.UpsertDocument(c.Request.Context(), doc); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("statement search index failed", "statement_id", statement.ID, "err", err)
	}
}

func (hc *HandlerContext) indexMileage(c *gin.Context, mileage models.MileageLog, suggestionContext string) {
	if hc.search == nil {
		return
	}
	doc := searchpkg.SearchDocumentFromMileage(tenantIDFromContext(c), searchpkg.MileageData{
		ID:                mileage.ID,
		EntityID:          mileage.EntityID,
		Date:              mileage.Date,
		DistanceMiles:     mileage.DistanceMiles,
		StartLocation:     mileage.StartLocation,
		EndLocation:       mileage.EndLocation,
		Purpose:           mileage.Purpose,
		SuggestionContext: suggestionContext,
		CreatedAt:         mileage.CreatedAt,
		UpdatedAt:         mileage.UpdatedAt,
	})
	if err := hc.search.UpsertDocument(c.Request.Context(), doc); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("mileage search index failed", "mileage_id", mileage.ID, "err", err)
	}
}

func (hc *HandlerContext) deleteSearchDocument(c *gin.Context, id string, attrs ...any) {
	if hc.search == nil {
		return
	}
	if err := hc.search.DeleteDocument(c.Request.Context(), id); err != nil && !errors.Is(err, db.ErrUnavailable) {
		args := append([]any{"document_id", id, "err", err}, attrs...)
		slog.Warn("search document delete failed", args...)
	}
}

func searchDocumentResponse(match searchpkg.DocumentMatch) gin.H {
	doc := match.Document
	return gin.H{
		"id":           doc.ID,
		"kind":         doc.Kind,
		"object_id":    doc.ObjectID,
		"entity_id":    doc.EntityID,
		"account_id":   doc.AccountID,
		"account_name": doc.AccountName,
		"title":        doc.Title,
		"subtitle":     doc.Subtitle,
		"body":         doc.Body,
		"status":       doc.Status,
		"tags":         doc.Tags,
		"date":         doc.Date,
		"amount_cents": doc.AmountCents,
		"href":         doc.Href,
		"score":        match.Score,
	}
}

func splitSearchValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	out := []string{}
	for _, value := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
