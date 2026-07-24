package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

type createTransactionRequest struct {
	EntityID  string               `json:"entity_id"`
	Date      string               `json:"date"`
	Memo      string               `json:"memo"`
	ReceiptID string               `json:"receipt_id"`
	Lines     []transactionLineReq `json:"lines"`
}

type transactionLineReq struct {
	AccountID   string `json:"account_id"`
	DebitCents  int64  `json:"debit_cents"`
	CreditCents int64  `json:"credit_cents"`
}

func (hc *HandlerContext) handleTransactionCreate(c *gin.Context) {
	if hc.transactions == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)

	var req createTransactionRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || req.Date == "" || len(req.Lines) == 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}

	entries := make([]models.DraftEntry, 0, len(req.Lines))
	for _, line := range req.Lines {
		if line.AccountID == "" {
			respondError(c, http.StatusBadRequest, CodeMissingFields)
			return
		}
		if (line.DebitCents == 0) == (line.CreditCents == 0) {
			respondError(c, http.StatusBadRequest, CodeInvalidEntry)
			return
		}
		entries = append(entries, models.DraftEntry{
			AccountID:   line.AccountID,
			DebitCents:  line.DebitCents,
			CreditCents: line.CreditCents,
		})
	}

	tr, outEntries, err := hc.transactions.Create(c.Request.Context(), req.EntityID, date, req.Memo, req.ReceiptID, entries)
	if err != nil {
		if errors.Is(err, db.ErrReceiptAlreadyAttached) {
			respondError(c, http.StatusConflict, CodeReceiptAlreadyAttached)
			return
		}
		respondError(c, http.StatusBadRequest, CodeInvalidTransaction)
		return
	}
	hc.auditEvent(c.Request.Context(), req.EntityID, userID, "transaction", tr.ID, "transaction.created", nil, map[string]interface{}{
		"transaction_id": tr.ID,
		"receipt_id":     req.ReceiptID,
		"lines":          len(outEntries),
	})
	hc.indexTransaction(c, tr, outEntries)
	hc.metrics.transactionPosted(c.Request.Context(), "direct")

	c.JSON(http.StatusCreated, gin.H{
		"transaction": transactionResponse(tr),
		"entries":     entryResponses(outEntries),
	})
}

func (hc *HandlerContext) handleTransactionList(c *gin.Context) {
	if hc.transactions == nil || hc.entities == nil {
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

	rows, err := hc.transactions.List(c.Request.Context(), entityID, start, end, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, gin.H{
			"transaction": transactionResponse(row.Transaction),
			"entries":     entryResponses(row.Entries),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"rows":      resp,
	})
}

func (hc *HandlerContext) handleTransactionSearch(c *gin.Context) {
	if hc.search == nil {
		c.JSON(http.StatusOK, gin.H{"rows": []gin.H{}})
		return
	}
	entityID := c.Query("entity_id")
	query := strings.TrimSpace(c.Query("q"))
	if entityID == "" || query == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	limit := queryLimit(c, 20, 50)
	matches, err := hc.search.SearchTransactions(c.Request.Context(), searchpkg.TransactionQuery{
		TenantID: tenantIDFromContext(c),
		EntityID: entityID,
		Query:    query,
		Limit:    limit,
	})
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeSearchUnavailable)
		return
	}
	rows := make([]gin.H, 0, len(matches))
	for _, match := range matches {
		doc := match.Document
		rows = append(rows, gin.H{
			"transaction_id":    doc.TransactionID,
			"entity_id":         doc.EntityID,
			"date":              doc.Date,
			"memo":              doc.Memo,
			"description":       doc.Description,
			"account_ids":       doc.AccountIDs,
			"account_names":     doc.AccountNames,
			"account_role_tags": doc.AccountRoleTags,
			"amount_cents":      doc.AmountCents,
			"score":             match.Score,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id": entityID,
		"query":     query,
		"rows":      rows,
	})
}

func (hc *HandlerContext) handleTransactionSearchReindex(c *gin.Context) {
	if hc.search == nil || hc.searchSource == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := strings.TrimSpace(c.Query("entity_id"))
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	result, err := (searchpkg.Reindexer{
		Provider: hc.search,
		Source:   hc.searchSource,
	}).ReindexTransactions(c.Request.Context(), searchpkg.ReindexOptions{
		TenantID: tenantIDFromContext(c),
		EntityID: entityID,
	})
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeSearchReindexFailed)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (hc *HandlerContext) indexTransaction(c *gin.Context, tr models.Transaction, entries []models.Entry) {
	if hc.search == nil {
		return
	}
	doc := hc.transactionSearchDocument(c, tr, entries)
	if doc.TransactionID == "" {
		return
	}
	if err := hc.search.UpsertTransaction(c.Request.Context(), doc); err != nil {
		slog.Warn("transaction search index failed", "transaction_id", tr.ID, "err", err)
	}
	if err := hc.search.UpsertDocument(c.Request.Context(), searchpkg.SearchDocumentFromTransaction(doc)); err != nil {
		slog.Warn("transaction document search index failed", "transaction_id", tr.ID, "err", err)
	}
}

func (hc *HandlerContext) transactionSearchDocument(c *gin.Context, tr models.Transaction, entries []models.Entry) searchpkg.TransactionDocument {
	entryData := make([]searchpkg.EntryData, 0, len(entries))
	for _, entry := range entries {
		entryData = append(entryData, searchpkg.EntryData{
			AccountID:   entry.AccountID,
			DebitCents:  entry.DebitCents,
			CreditCents: entry.CreditCents,
		})
	}
	accountData := []searchpkg.AccountData{}
	if hc.accounts != nil && tr.EntityID != "" {
		if accounts, err := hc.accounts.ListForEntity(c.Request.Context(), tr.EntityID, 1000); err == nil {
			for _, account := range accounts {
				accountData = append(accountData, searchpkg.AccountData{
					ID:       account.ID,
					Name:     account.Name,
					RoleTags: account.RoleTags,
				})
			}
		}
	}
	return searchpkg.TransactionDocumentFromData(tenantIDFromContext(c), tr.ID, tr.EntityID, tr.Date, tr.Memo, entryData, accountData, tr.CreatedAt)
}
