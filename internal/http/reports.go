package httpapi

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/reporting"
)

func (hc *HandlerContext) handleReportGeneralLedger(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	rows, err := hc.reports.GeneralLedger(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id":  entityID,
		"start_date": start.Format("2006-01-02"),
		"end_date":   end.Format("2006-01-02"),
		"rows":       rows,
	})
}

func (hc *HandlerContext) handleReportProfitLoss(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	income := make([]gin.H, 0)
	expenses := make([]gin.H, 0)
	var incomeTotal, expenseTotal int64
	for _, row := range rows {
		switch reporting.NormalizeType(row.AccountType) {
		case "income":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			incomeTotal += amount
			income = append(income, reportLine(row, amount))
		case "expense":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			expenseTotal += amount
			expenses = append(expenses, reportLine(row, amount))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"entity_id":        entityID,
		"start_date":       start.Format("2006-01-02"),
		"end_date":         end.Format("2006-01-02"),
		"income":           income,
		"expenses":         expenses,
		"net_income_cents": incomeTotal - expenseTotal,
	})
}

func (hc *HandlerContext) handleReportBalanceSheet(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	assets := make([]gin.H, 0)
	liabilities := make([]gin.H, 0)
	equity := make([]gin.H, 0)
	var assetTotal, liabilityTotal, equityTotal int64
	for _, row := range rows {
		switch reporting.NormalizeType(row.AccountType) {
		case "asset":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			assetTotal += amount
			assets = append(assets, reportLine(row, amount))
		case "liability":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			liabilityTotal += amount
			liabilities = append(liabilities, reportLine(row, amount))
		case "equity":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			equityTotal += amount
			equity = append(equity, reportLine(row, amount))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"entity_id":               entityID,
		"start_date":              start.Format("2006-01-02"),
		"end_date":                end.Format("2006-01-02"),
		"assets":                  assets,
		"liabilities":             liabilities,
		"equity":                  equity,
		"total_assets_cents":      assetTotal,
		"total_liabilities_cents": liabilityTotal,
		"total_equity_cents":      equityTotal,
	})
}

// handleReportTrialBalance lists each account's net balance in the debit or credit column. A balanced
// ledger nets to zero across all accounts, so the column totals match.
func (hc *HandlerContext) handleReportTrialBalance(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	lines := make([]gin.H, 0, len(rows))
	var totalDebit, totalCredit int64
	for _, row := range rows {
		net := row.DebitCents - row.CreditCents
		if net == 0 {
			continue
		}
		debit, credit := reporting.SplitDebitCredit(net)
		totalDebit += debit
		totalCredit += credit
		lines = append(lines, gin.H{
			"account_id":   row.AccountID,
			"account_name": row.AccountName,
			"account_type": row.AccountType,
			"debit_cents":  debit,
			"credit_cents": credit,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id":          entityID,
		"start_date":         start.Format("2006-01-02"),
		"end_date":           end.Format("2006-01-02"),
		"lines":              lines,
		"total_debit_cents":  totalDebit,
		"total_credit_cents": totalCredit,
		"balanced":           totalDebit == totalCredit,
	})
}

// handleReportVendorPayments lists total expense paid per vendor in a period and flags those at/above the
// 1099-NEC reporting threshold (default $600). Advisory: a preparer confirms payment method and eligibility.
func (hc *HandlerContext) handleReportVendorPayments(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	threshold := int64(60000)
	if v := c.Query("min_cents"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			threshold = n
		}
	}
	rows, err := hc.reports.VendorPayments(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		out = append(out, gin.H{
			"vendor_id":   row.VendorID,
			"vendor_name": row.VendorName,
			"tax_id":      row.TaxID,
			"total_cents": row.TotalCents,
			"needs_1099":  row.TotalCents >= threshold,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"entity_id":       entityID,
		"start_date":      start.Format("2006-01-02"),
		"end_date":        end.Format("2006-01-02"),
		"threshold_cents": threshold,
		"rows":            out,
	})
}

func (hc *HandlerContext) handleExportTransactionsCSV(c *gin.Context) {
	entityID, start, end, ok := hc.parseReportParams(c)
	if !ok {
		return
	}
	rows, err := hc.reports.TransactionsForExport(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=transactions.csv")

	w := csv.NewWriter(c.Writer)
	_ = w.Write(transactionExportHeader())
	for _, row := range rows {
		_ = w.Write(transactionExportRow(row))
	}
	w.Flush()
}

func transactionExportHeader() []string {
	return []string{"date", "transaction_id", "memo", "account_id", "account_name", "account_type", "debit_cents", "credit_cents", "source_kind", "source_id", "source_name", "source_row", "source_vendor", "source_hash"}
}

func transactionExportRow(row db.GeneralLedgerRow) []string {
	sourceRow := ""
	if row.SourceRow > 0 {
		sourceRow = strconv.Itoa(row.SourceRow)
	}
	return []string{
		row.Date.Format("2006-01-02"),
		row.TransactionID,
		row.Memo,
		row.AccountID,
		row.AccountName,
		row.AccountType,
		strconv.FormatInt(row.DebitCents, 10),
		strconv.FormatInt(row.CreditCents, 10),
		row.SourceKind,
		row.SourceID,
		row.SourceName,
		sourceRow,
		row.SourceVendor,
		row.SourceHash,
	}
}

func (hc *HandlerContext) parseReportParams(c *gin.Context) (string, time.Time, time.Time, bool) {
	if hc.reports == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return "", time.Time{}, time.Time{}, false
	}
	entityID := c.Query("entity_id")
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")
	if entityID == "" || startStr == "" || endStr == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return "", time.Time{}, time.Time{}, false
	}
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return "", time.Time{}, time.Time{}, false
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return "", time.Time{}, time.Time{}, false
	}
	return entityID, start, end, true
}

func reportLine(row db.TrialBalanceRow, amount int64) gin.H {
	return gin.H{
		"account_id":   row.AccountID,
		"account_name": row.AccountName,
		"account_type": row.AccountType,
		"amount_cents": amount,
	}
}
