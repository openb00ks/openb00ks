package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/reporting"
)

// naturalAccountBalance applies the account's normal balance side: assets and expenses carry a debit
// balance (debit − credit); liabilities, equity, and income carry a credit balance (credit − debit).
func naturalAccountBalance(accountType string, debit, credit int64) int64 {
	return reporting.NormalBalance(accountType, debit, credit)
}

// handleAccountBalances returns the current natural balance for every account of an entity (0 for accounts
// with no postings), for the accounts list.
func (hc *HandlerContext) handleAccountBalances(c *gin.Context) {
	if hc.reports == nil {
		hc.notImplemented(c)
		return
	}
	entityID := c.Param("id")
	rows, err := hc.reports.AccountBalances(c.Request.Context(), entityID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	balances := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		balances = append(balances, gin.H{
			"account_id":    row.AccountID,
			"balance_cents": naturalAccountBalance(row.AccountType, row.DebitCents, row.CreditCents),
		})
	}
	c.JSON(http.StatusOK, gin.H{"balances": balances})
}

// handleAccountTransactions returns a single account (name/type/code), its current balance, and its posted
// journal lines newest-first — the account detail view.
func (hc *HandlerContext) handleAccountTransactions(c *gin.Context) {
	if hc.reports == nil || hc.accounts == nil {
		hc.notImplemented(c)
		return
	}
	accountID := c.Param("id")
	account, err := hc.accounts.GetByID(c.Request.Context(), accountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound)
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	limit := queryLimit(c, 100, 1000)
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n >= 0 {
			offset = n
		}
	}
	// Fetch one extra to tell the client whether an older page exists (running balance stays correct because
	// pages are contiguous newest→older).
	ledger, err := hc.reports.AccountLedger(c.Request.Context(), account.EntityID, accountID, limit+1, offset)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	hasMore := len(ledger) > limit
	if hasMore {
		ledger = ledger[:limit]
	}
	balances, err := hc.reports.AccountBalances(c.Request.Context(), account.EntityID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	var balanceCents int64
	for _, b := range balances {
		if b.AccountID == accountID {
			balanceCents = naturalAccountBalance(b.AccountType, b.DebitCents, b.CreditCents)
			break
		}
	}

	rows := make([]gin.H, 0, len(ledger))
	for _, r := range ledger {
		rows = append(rows, gin.H{
			"transaction_id": r.TransactionID,
			"date":           r.Date.Format("2006-01-02"),
			"memo":           r.Memo,
			"debit_cents":    r.DebitCents,
			"credit_cents":   r.CreditCents,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"account": gin.H{
			"id":        account.ID,
			"entity_id": account.EntityID,
			"name":      account.Name,
			"type":      account.Type,
			"code":      account.Code,
		},
		"balance_cents": balanceCents,
		"rows":          rows,
		"has_more":      hasMore,
	})
}
