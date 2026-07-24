package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/pipeline"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

// vendorRequest is the create/update body. Name is required; the rest are optional. NormalizedName is
// derived server-side (never client-supplied) so it stays consistent with the pipeline's matcher.
type vendorRequest struct {
	EntityID         string `json:"entity_id"`
	Name             string `json:"name"`
	MatchPattern     string `json:"match_pattern"`
	TaxID            string `json:"tax_id"`
	Website          string `json:"website"`
	DefaultAccountID string `json:"default_account_id"`
}

func (hc *HandlerContext) handleVendorsList(c *gin.Context) {
	if hc.vendors == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	limit := queryLimit(c, 200, 1000)
	rows, err := hc.vendors.ListForEntity(c.Request.Context(), entityID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	resp := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, vendorResponse(row))
	}
	c.JSON(http.StatusOK, gin.H{"rows": resp})
}

func (hc *HandlerContext) handleVendorsGet(c *gin.Context) {
	if hc.vendors == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	vendor, err := hc.vendors.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, vendorResponse(vendor))
}

func (hc *HandlerContext) handleVendorsCreate(c *gin.Context) {
	if hc.vendors == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req vendorRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if req.EntityID == "" || strings.TrimSpace(req.Name) == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	vendor, err := hc.vendors.Create(c.Request.Context(), vendorFromRequest(req.EntityID, req))
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hc.indexVendor(c, vendor)
	c.JSON(http.StatusCreated, vendorResponse(vendor))
}

func (hc *HandlerContext) handleVendorsUpdate(c *gin.Context) {
	if hc.vendors == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	var req vendorRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return
	}
	vendor, err := hc.vendors.Update(c.Request.Context(), c.Param("id"), vendorFromRequest(req.EntityID, req))
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			respondError(c, http.StatusNotFound, CodeNotFound)
		case errors.Is(err, db.ErrConflict):
			respondError(c, http.StatusConflict, CodeDuplicateVendor)
		default:
			respondError(c, http.StatusInternalServerError, CodeInternalError)
		}
		return
	}
	hc.indexVendor(c, vendor)
	c.JSON(http.StatusOK, vendorResponse(vendor))
}

func (hc *HandlerContext) handleVendorsDelete(c *gin.Context) {
	if hc.vendors == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return
	}
	if err := hc.vendors.Delete(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if hc.search != nil {
		id := c.Param("id")
		if err := hc.search.DeleteDocument(c.Request.Context(), "vendor_"+id); err != nil && !errors.Is(err, db.ErrUnavailable) {
			slog.Warn("vendor search delete failed", "vendor_id", id, "err", err)
		}
		if err := hc.search.DeleteVendor(c.Request.Context(), id); err != nil && !errors.Is(err, db.ErrUnavailable) {
			slog.Warn("vendor retrieval-index delete failed", "vendor_id", id, "err", err)
		}
	}
	c.Status(http.StatusNoContent)
}

// indexVendor best-effort refreshes a vendor's search documents: the _documents entry (human global
// search) and the _vendors entry (pipeline retrieval). Keeping the latter current on edits matters —
// retrieval reads the default account straight from that doc, so a stale one would misfile receipts.
// Search is an augmentation: its unavailability never fails the write (a reindex backfills).
func (hc *HandlerContext) indexVendor(c *gin.Context, vendor db.Vendor) {
	if hc.search == nil {
		return
	}
	ctx := c.Request.Context()
	tenantID := tenantIDFromContext(c)

	if err := hc.search.UpsertDocument(ctx, searchpkg.SearchDocumentFromVendor(tenantID, searchpkg.VendorData{
		ID:               vendor.ID,
		EntityID:         vendor.EntityID,
		Name:             vendor.Name,
		MatchPattern:     vendor.MatchPattern,
		Website:          vendor.Website,
		DefaultAccountID: vendor.DefaultAccountID,
	})); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("vendor search index failed", "vendor_id", vendor.ID, "err", err)
	}

	var aliases []string
	if hc.vendorAliases != nil {
		aliases, _ = hc.vendorAliases.ListNormalized(ctx, vendor.ID)
	}
	lastSeen := int64(0)
	if !vendor.LastSeen.IsZero() {
		lastSeen = vendor.LastSeen.Unix()
	}
	if err := hc.search.UpsertVendor(ctx, searchpkg.VendorDocumentFromData(tenantID, searchpkg.VendorData{
		ID:               vendor.ID,
		EntityID:         vendor.EntityID,
		Name:             vendor.Name,
		MatchPattern:     vendor.MatchPattern,
		TaxID:            vendor.TaxID,
		Website:          vendor.Website,
		DefaultAccountID: vendor.DefaultAccountID,
		Aliases:          aliases,
		ReceiptCount:     int32(vendor.ReceiptCount),
		LastSeenUnix:     lastSeen,
	})); err != nil && !errors.Is(err, db.ErrUnavailable) {
		slog.Warn("vendor retrieval-index failed", "vendor_id", vendor.ID, "err", err)
	}
}

// vendorFromRequest maps the API body to a db.Vendor, deriving normalized_name from the display name so
// it matches what the receipt pipeline stores (pipeline.NormalizeVendorName).
func vendorFromRequest(entityID string, req vendorRequest) db.Vendor {
	name := strings.TrimSpace(req.Name)
	return db.Vendor{
		EntityID:         entityID,
		Name:             name,
		NormalizedName:   pipeline.NormalizeVendorName(name),
		MatchPattern:     strings.TrimSpace(req.MatchPattern),
		TaxID:            strings.TrimSpace(req.TaxID),
		Website:          strings.TrimSpace(req.Website),
		DefaultAccountID: strings.TrimSpace(req.DefaultAccountID),
	}
}

func vendorResponse(v db.Vendor) gin.H {
	h := gin.H{
		"id":                 v.ID,
		"entity_id":          v.EntityID,
		"name":               v.Name,
		"normalized_name":    v.NormalizedName,
		"match_pattern":      v.MatchPattern,
		"tax_id":             v.TaxID,
		"website":            v.Website,
		"default_account_id": v.DefaultAccountID,
		"receipt_count":      v.ReceiptCount,
	}
	if !v.LastSeen.IsZero() {
		h["last_seen"] = v.LastSeen.Format("2006-01-02T15:04:05Z07:00")
	}
	return h
}
