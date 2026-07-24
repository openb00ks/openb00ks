package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/vendormemo"
)

type receiptVendorRequest struct {
	VendorID string `json:"vendor_id"`
}

// handleReceiptSetVendor re-points a receipt at a different vendor — the reviewer correcting a mis-matched
// or mis-created vendor before posting. The correction sticks (resolved_vendor_id), and because the raw
// receipt string is left intact, posting then trains the *right* vendor. A blank vendor_id clears it.
func (hc *HandlerContext) handleReceiptSetVendor(c *gin.Context) {
	if hc.receiptStore == nil || hc.vendors == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	ctx := c.Request.Context()
	receiptID := c.Param("id")
	var req receiptVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	// A chosen vendor must belong to the receipt's own entity (no cross-entity re-pointing).
	if strings.TrimSpace(req.VendorID) != "" {
		entityID, err := hc.receiptStore.GetEntityID(ctx, receiptID)
		if err != nil {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		v, err := hc.vendors.GetByID(ctx, req.VendorID)
		if err != nil || v.EntityID != entityID {
			respondError(c, http.StatusBadRequest, CodeInvalidVendor)
			return
		}
	}
	if err := hc.receiptStore.UpdateResolvedVendorID(ctx, receiptID, req.VendorID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.Status(http.StatusNoContent)
}

// learnFromPostedReceipt closes the reviewer feedback loop: when a human posts a receipt-derived draft,
// the expense account they settled on is fed back to the vendor the pipeline resolved — so the next
// receipt from that vendor pre-fills the reviewer's choice, not the AI's, and the raw string is reinforced
// as an alias. Best-effort: it never affects the post itself (the transaction is already committed), and
// it no-ops for drafts with no resolved vendor (bank imports, manual entries, pre-feedback receipts).
func (hc *HandlerContext) learnFromPostedReceipt(c *gin.Context, entityID, receiptID string, entries []models.Entry) {
	if hc.vendors == nil || hc.receiptStore == nil || hc.accounts == nil {
		return
	}
	ctx := c.Request.Context()
	receipt, err := hc.receiptStore.GetByID(ctx, receiptID)
	if err != nil || strings.TrimSpace(receipt.ResolvedVendorID) == "" {
		return
	}
	account := hc.chosenExpenseAccount(ctx, entityID, entries)
	if account == "" {
		return
	}
	memo := vendormemo.Deps{Vendors: hc.vendors, Aliases: hc.vendorAliases, Search: hc.search}
	if err := memo.LearnAccount(ctx, tenantIDFromContext(c), entityID, receipt.ResolvedVendorID, receipt.ResolvedVendorRaw, account); err != nil {
		slog.Warn("vendor feedback learn failed", "receipt_id", receiptID, "vendor_id", receipt.ResolvedVendorID, "err", err)
	}
}

// chosenExpenseAccount resolves the entity's account types and returns the reviewer's chosen expense
// account from the posted entries. Empty when the draft has no expense debit (e.g. a transfer).
func (hc *HandlerContext) chosenExpenseAccount(ctx context.Context, entityID string, entries []models.Entry) string {
	accounts, err := hc.accounts.ListForEntity(ctx, entityID, 1000)
	if err != nil {
		return ""
	}
	typeByID := make(map[string]string, len(accounts))
	for _, a := range accounts {
		typeByID[a.ID] = a.Type
	}
	return pickExpenseAccount(entries, typeByID)
}

// pickExpenseAccount returns the account id of the largest debit to an expense-type account across the
// posted entries — the entry that carries the reviewer's categorization. Pure: the debit/type inputs make
// it independent of the DB, so the "which line did the human choose" rule is unit-testable. Empty when no
// entry debits an expense account.
func pickExpenseAccount(entries []models.Entry, typeByID map[string]string) string {
	best := ""
	var bestAmt int64
	for _, e := range entries {
		if e.DebitCents <= 0 || !strings.EqualFold(typeByID[e.AccountID], "expense") {
			continue
		}
		if e.DebitCents > bestAmt {
			best = e.AccountID
			bestAmt = e.DebitCents
		}
	}
	return best
}
