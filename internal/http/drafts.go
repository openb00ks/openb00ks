package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

func (hc *HandlerContext) handleDraftGet(c *gin.Context) {
	if hc.drafts == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	_ = userIDFromContext(c)
	receiptID := c.Param("id")
	draft, err := hc.drafts.GetByReceiptID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, draftResponse(draft))
}

func (hc *HandlerContext) handleDraftPost(c *gin.Context) {
	if hc.drafts == nil || hc.transactions == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)
	receiptID := c.Param("id")
	draft, err := hc.drafts.GetByReceiptID(c.Request.Context(), receiptID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	tr, entries, err := hc.transactions.CreateFromDraft(c.Request.Context(), draft, receiptID)
	if err != nil {
		if errors.Is(err, db.ErrReceiptAlreadyAttached) {
			respondError(c, http.StatusConflict, CodeReceiptAlreadyAttached)
			return
		}
		respondError(c, http.StatusBadRequest, CodeInvalidDraft)
		return
	}
	hc.auditEvent(c.Request.Context(), draft.EntityID, userID, "transaction", tr.ID, "transaction.posted", map[string]interface{}{
		"draft_id": draft.ID,
	}, map[string]interface{}{
		"transaction_id": tr.ID,
		"receipt_id":     receiptID,
	})
	hc.indexTransaction(c, tr, entries)
	hc.learnFromPostedReceipt(c, draft.EntityID, receiptID, entries)
	hc.metrics.transactionPosted(c.Request.Context(), "receipt")
	c.JSON(http.StatusCreated, gin.H{
		"transaction": transactionResponse(tr),
		"entries":     entryResponses(entries),
	})
}

type updateDraftRequest struct {
	Date    string             `json:"date"`
	Memo    string             `json:"memo"`
	Entries []updateDraftEntry `json:"entries"`
}

type updateDraftEntry struct {
	AccountID   string `json:"account_id"`
	DebitCents  int64  `json:"debit_cents"`
	CreditCents int64  `json:"credit_cents"`
}

func (hc *HandlerContext) handleDraftUpdate(c *gin.Context) {
	if hc.drafts == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	userID := userIDFromContext(c)
	receiptID := c.Param("id")
	var req updateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.Date == "" || len(req.Entries) == 0 {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return
	}

	entries := make([]models.DraftEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		if e.AccountID == "" {
			respondError(c, http.StatusBadRequest, CodeMissingFields)
			return
		}
		if (e.DebitCents == 0) == (e.CreditCents == 0) {
			respondError(c, http.StatusBadRequest, CodeInvalidEntry)
			return
		}
		entries = append(entries, models.DraftEntry{
			AccountID:   e.AccountID,
			DebitCents:  e.DebitCents,
			CreditCents: e.CreditCents,
		})
	}

	updated, err := hc.drafts.UpdateDraft(c.Request.Context(), receiptID, date, req.Memo, entries)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.auditEvent(c.Request.Context(), updated.EntityID, userID, "draft", updated.ID, "draft.updated", nil, map[string]interface{}{
		"draft_id": updated.ID,
		"entries":  len(updated.Entries),
	})
	c.JSON(http.StatusOK, draftResponse(updated))
}

func draftResponse(draft models.DraftTransaction) gin.H {
	entries := make([]gin.H, 0, len(draft.Entries))
	for _, e := range draft.Entries {
		entries = append(entries, gin.H{
			"id":           e.ID,
			"account_id":   e.AccountID,
			"debit_cents":  e.DebitCents,
			"credit_cents": e.CreditCents,
		})
	}
	memo := draft.Memo
	return gin.H{
		"id":         draft.ID,
		"receipt_id": draft.ReceiptID,
		"entity_id":  draft.EntityID,
		"date":       draft.Date.Format("2006-01-02"),
		"memo":       memo,
		"created_at": draft.CreatedAt.Format(time.RFC3339),
		"updated_at": draft.UpdatedAt.Format(time.RFC3339),
		"entries":    entries,
	}
}

func transactionResponse(tr models.Transaction) gin.H {
	return gin.H{
		"id":         tr.ID,
		"entity_id":  tr.EntityID,
		"date":       tr.Date.Format("2006-01-02"),
		"memo":       tr.Memo,
		"created_at": tr.CreatedAt.Format(time.RFC3339),
	}
}

func entryResponses(entries []models.Entry) []gin.H {
	out := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		out = append(out, gin.H{
			"id":             e.ID,
			"transaction_id": e.TransactionID,
			"account_id":     e.AccountID,
			"debit_cents":    e.DebitCents,
			"credit_cents":   e.CreditCents,
		})
	}
	return out
}
