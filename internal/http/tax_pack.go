package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
	"github.com/openb00ks/openb00ks/internal/reporting"
)

type taxException struct {
	SourceID   string
	SourceName string
	Kind       string
	Status     string
	Issue      string
	RowIndex   string
	Vendor     string
	Amount     string
}

type taxActionItem struct {
	Kind     string
	Label    string
	Count    int
	Href     string
	Priority int
}

type taxBlockingSummary struct {
	SourceID      string
	SourceName    string
	Kind          string
	Status        string
	IssueCount    int
	UnmappedRows  int
	DuplicateRows int
	NotPosted     int
	ParseErrors   int
	FirstRowIndex string
	Href          string
}

type taxUseProfileState struct {
	TaxYear                         int
	Status                          string
	HomeOfficeSqFt                  *int
	HomeTotalSqFt                   *int
	HomeUtilitiesBusinessUsePercent *int
	CellPhoneBusinessUsePercent     *int
	HomeInternetBusinessUsePercent  *int
	Href                            string
}

type taxAccountRoleCoverageState struct {
	UtilitiesCount int
	CellPhoneCount int
	InternetCount  int
	Href           string
}

func (hc *HandlerContext) handleReportTaxReadiness(c *gin.Context) {
	entityID, start, end, label, ok := hc.parseTaxPackParams(c)
	if !ok {
		return
	}
	taxYear := start.Year()
	exceptions, importSummary, err := hc.taxImportDiagnostics(c, entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	exceptions = append(exceptions, hc.taxUseProfileExceptions(c, entityID, taxYear)...)
	exceptions = append(exceptions, hc.taxAccountRoleExceptions(c, entityID, taxYear)...)
	exceptions = append(exceptions, hc.taxStatementExceptions(c, entityID, start, end)...)
	rows, err := hc.reports.TransactionsForExport(c.Request.Context(), entityID, start, end)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	exceptionRows := make([]gin.H, 0, len(exceptions))
	for _, item := range exceptions {
		exceptionRows = append(exceptionRows, taxExceptionResponse(item))
	}
	actionRows := make([]gin.H, 0)
	for _, item := range taxActionItems(exceptions) {
		actionRows = append(actionRows, taxActionResponse(item))
	}
	taxUseProfile := hc.taxUseProfileResponse(c, entityID, taxYear)
	accountRoleCoverage := hc.taxAccountRoleCoverageResponse(c, entityID)
	c.JSON(http.StatusOK, gin.H{
		"entity_id":               entityID,
		"start_date":              start.Format("2006-01-02"),
		"end_date":                end.Format("2006-01-02"),
		"label":                   label,
		"ready":                   len(exceptions) == 0,
		"exception_count":         len(exceptions),
		"posted_entry_line_count": len(rows),
		"import_summary":          importSummaryRows(importSummary),
		"blocking_summary":        blockingSummaryRows(exceptions),
		"tax_use_profile":         taxUseProfile,
		"account_role_coverage":   accountRoleCoverage,
		"actions":                 actionRows,
		"exceptions":              exceptionRows,
	})
}

func (hc *HandlerContext) handleExportTaxPack(c *gin.Context) {
	entityID, start, end, label, ok := hc.parseTaxPackParams(c)
	if !ok {
		return
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	exceptions, importSummary, err := hc.taxImportDiagnostics(c, entityID, start, end)
	if err != nil {
		_ = zw.Close()
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	exceptions = append(exceptions, hc.taxUseProfileExceptions(c, entityID, start.Year())...)
	exceptions = append(exceptions, hc.taxAccountRoleExceptions(c, entityID, start.Year())...)
	exceptions = append(exceptions, hc.taxStatementExceptions(c, entityID, start, end)...)

	if err := hc.writeTaxPackFiles(c, zw, entityID, start, end, label, exceptions, importSummary); err != nil {
		_ = zw.Close()
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}
	if err := zw.Close(); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError)
		return
	}

	filename := fmt.Sprintf("tax-pack-%s-%s.zip", label, time.Now().Format("20060102"))
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func taxExceptionResponse(item taxException) gin.H {
	return gin.H{
		"source_id":    item.SourceID,
		"source_name":  item.SourceName,
		"kind":         item.Kind,
		"status":       item.Status,
		"issue":        item.Issue,
		"row_index":    item.RowIndex,
		"vendor":       item.Vendor,
		"amount_cents": item.Amount,
	}
}

func taxActionResponse(item taxActionItem) gin.H {
	return gin.H{
		"kind":     item.Kind,
		"label":    item.Label,
		"count":    item.Count,
		"href":     item.Href,
		"priority": item.Priority,
	}
}

func taxExceptionHref(item taxException) string {
	if item.SourceID == "" {
		return ""
	}
	switch {
	case item.Kind == "entity":
		return "/settings/entity"
	case item.Kind == "account_statement":
		return "/statements"
	case item.Kind == "receipt" || item.Issue == "not posted":
		return "/receipts/" + item.SourceID
	case item.RowIndex != "":
		return "/imports/" + item.SourceID + "#row-" + item.RowIndex
	default:
		return "/imports/" + item.SourceID
	}
}

func (hc *HandlerContext) taxAccountRoleCoverageResponse(c *gin.Context, entityID string) gin.H {
	coverage := hc.loadAccountRoleCoverage(c, entityID)
	return gin.H{
		"utilities_count":  coverage.UtilitiesCount,
		"cell_phone_count": coverage.CellPhoneCount,
		"internet_count":   coverage.InternetCount,
		"href":             coverage.Href,
	}
}

func (hc *HandlerContext) loadAccountRoleCoverage(c *gin.Context, entityID string) taxAccountRoleCoverageState {
	coverage := taxAccountRoleCoverageState{Href: "/accounts"}
	if hc == nil || hc.accounts == nil {
		return coverage
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	accounts, err := hc.accounts.ListForEntity(ctx, entityID, 1000)
	if err != nil {
		return coverage
	}
	for _, account := range accounts {
		for _, roleTag := range account.RoleTags {
			switch roleTag {
			case models.AccountRoleUtilities:
				coverage.UtilitiesCount++
			case models.AccountRoleCellPhone:
				coverage.CellPhoneCount++
			case models.AccountRoleInternet:
				coverage.InternetCount++
			}
		}
	}
	return coverage
}

func (hc *HandlerContext) taxAccountRoleExceptions(c *gin.Context, entityID string, taxYear int) []taxException {
	profile := hc.loadTaxUseProfile(c, entityID, taxYear)
	if profile.Status == "missing" {
		return nil
	}
	coverage := hc.loadAccountRoleCoverage(c, entityID)
	exceptions := make([]taxException, 0, 3)
	if profile.HomeUtilitiesBusinessUsePercent != nil && coverage.UtilitiesCount == 0 {
		exceptions = append(exceptions, taxException{
			SourceID:   entityID,
			SourceName: "Entity settings",
			Kind:       "entity",
			Status:     profile.Status,
			Issue:      "missing utilities tagged account",
		})
	}
	if profile.CellPhoneBusinessUsePercent != nil && coverage.CellPhoneCount == 0 {
		exceptions = append(exceptions, taxException{
			SourceID:   entityID,
			SourceName: "Entity settings",
			Kind:       "entity",
			Status:     profile.Status,
			Issue:      "missing cell phone tagged account",
		})
	}
	if profile.HomeInternetBusinessUsePercent != nil && coverage.InternetCount == 0 {
		exceptions = append(exceptions, taxException{
			SourceID:   entityID,
			SourceName: "Entity settings",
			Kind:       "entity",
			Status:     profile.Status,
			Issue:      "missing internet tagged account",
		})
	}
	return exceptions
}

func (hc *HandlerContext) accountRoleTagRows(c *gin.Context, entityID string) [][]string {
	rows := [][]string{{"account_id", "account_name", "account_type", "role_tags", "href"}}
	if hc == nil || hc.accounts == nil {
		return rows
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	accounts, err := hc.accounts.ListForEntity(ctx, entityID, 1000)
	if err != nil {
		return rows
	}
	for _, account := range accounts {
		rows = append(rows, []string{
			account.ID,
			account.Name,
			account.Type,
			strings.Join(account.RoleTags, ","),
			"/accounts",
		})
	}
	return rows
}

func importSummaryRows(rows [][]string) []gin.H {
	if len(rows) <= 1 {
		return []gin.H{}
	}
	out := make([]gin.H, 0, len(rows)-1)
	for _, row := range rows[1:] {
		padded := append(row, make([]string, max(0, 14-len(row)))...)
		out = append(out, gin.H{
			"import_id":            padded[0],
			"file":                 padded[1],
			"status":               padded[2],
			"row_count":            padded[3],
			"parsed_rows":          padded[4],
			"error_rows":           padded[5],
			"outflow_cents":        padded[6],
			"inflow_cents":         padded[7],
			"posted_outflow_cents": padded[8],
			"posted_inflow_cents":  padded[9],
			"mapped_rows":          padded[10],
			"posted_rows":          padded[11],
			"unposted_rows":        padded[12],
			"duplicate_rows":       padded[13],
		})
	}
	return out
}

func (hc *HandlerContext) parseTaxPackParams(c *gin.Context) (string, time.Time, time.Time, string, bool) {
	if hc.reports == nil || hc.entities == nil {
		respondError(c, http.StatusNotImplemented, CodeNotImplemented)
		return "", time.Time{}, time.Time{}, "", false
	}
	entityID := c.Query("entity_id")
	if entityID == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return "", time.Time{}, time.Time{}, "", false
	}
	if yearStr := strings.TrimSpace(c.Query("year")); yearStr != "" {
		year, err := strconv.Atoi(yearStr)
		if err != nil || year < 1900 || year > 3000 {
			respondError(c, http.StatusBadRequest, CodeInvalidYear)
			return "", time.Time{}, time.Time{}, "", false
		}
		start, end := hc.fiscalYearRange(c.Request.Context(), entityID, year)
		return entityID, start, end, yearStr, true
	}
	startStr := c.Query("start_date")
	endStr := c.Query("end_date")
	if startStr == "" || endStr == "" {
		respondError(c, http.StatusBadRequest, CodeMissingFields)
		return "", time.Time{}, time.Time{}, "", false
	}
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return "", time.Time{}, time.Time{}, "", false
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidDate)
		return "", time.Time{}, time.Time{}, "", false
	}
	return entityID, start, end, startStr + "-to-" + endStr, true
}

type entityByIDStore interface {
	GetByID(ctx context.Context, entityID string) (models.Entity, error)
}

func (hc *HandlerContext) fiscalYearRange(ctx context.Context, entityID string, year int) (time.Time, time.Time) {
	month := 1
	day := 1
	if getter, ok := hc.entities.(entityByIDStore); ok {
		if entity, err := getter.GetByID(ctx, entityID); err == nil {
			if entity.FiscalYearStartMonth >= 1 && entity.FiscalYearStartMonth <= 12 {
				month = entity.FiscalYearStartMonth
			}
			if entity.FiscalYearStartDay >= 1 && entity.FiscalYearStartDay <= 31 {
				day = entity.FiscalYearStartDay
			}
		}
	}
	start := validFiscalYearStart(year, month, day)
	return start, start.AddDate(1, 0, -1)
}

func validFiscalYearStart(year, month, day int) time.Time {
	for day > 28 {
		start := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if start.Month() == time.Month(month) && start.Day() == day {
			return start
		}
		day--
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func (hc *HandlerContext) writeTaxPackFiles(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time, label string, exceptions []taxException, importSummary [][]string) error {
	if err := hc.writeProfitLossCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeBalanceSheetCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeTrialBalanceCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeGeneralLedgerCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeTransactionsCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeMileageCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := hc.writeVendorPaymentsCSV(c, zw, entityID, start, end); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "import-summary.csv", importSummary); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "exceptions.csv", taxExceptionsRows(exceptions)); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "review-actions.csv", taxActionRows(taxActionItems(exceptions))); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "blocking-summary.csv", taxBlockingSummaryRows(exceptions)); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "home-use-allocation.csv", hc.taxUseAllocationRows(c, entityID, start.Year())); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "account-role-tags.csv", hc.accountRoleTagRows(c, entityID)); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "statement-reconciliation.csv", hc.taxStatementRows(c, entityID, start, end)); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "prep-checklist.csv", hc.taxPrepChecklistRows(c, entityID, start, end, exceptions)); err != nil {
		return err
	}
	if err := writeCSVFile(zw, "prepared-package.csv", hc.taxPreparedPackageRows(c, entityID, start, end, exceptions, importSummary)); err != nil {
		return err
	}
	return writeTextFile(zw, "README.md", taxPackReadme(entityID, start, end, label, len(exceptions)))
}

func (hc *HandlerContext) writeProfitLossCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	out := [][]string{{"section", "account_id", "account_name", "account_type", "amount_cents"}}
	var incomeTotal, expenseTotal int64
	for _, row := range rows {
		switch reporting.NormalizeType(row.AccountType) {
		case "income":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			incomeTotal += amount
			out = append(out, []string{"income", row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(amount, 10)})
		case "expense":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			expenseTotal += amount
			out = append(out, []string{"expense", row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(amount, 10)})
		}
	}
	out = append(out, []string{"net_income", "", "", "", strconv.FormatInt(incomeTotal-expenseTotal, 10)})
	return writeCSVFile(zw, "profit-loss.csv", out)
}

func (hc *HandlerContext) writeBalanceSheetCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	out := [][]string{{"section", "account_id", "account_name", "account_type", "amount_cents"}}
	var assetTotal, liabilityTotal, equityTotal int64
	for _, row := range rows {
		switch reporting.NormalizeType(row.AccountType) {
		case "asset":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			assetTotal += amount
			out = append(out, []string{"asset", row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(amount, 10)})
		case "liability":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			liabilityTotal += amount
			out = append(out, []string{"liability", row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(amount, 10)})
		case "equity":
			amount := reporting.NormalBalance(row.AccountType, row.DebitCents, row.CreditCents)
			equityTotal += amount
			out = append(out, []string{"equity", row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(amount, 10)})
		}
	}
	out = append(out, []string{"total_assets", "", "", "", strconv.FormatInt(assetTotal, 10)})
	out = append(out, []string{"total_liabilities", "", "", "", strconv.FormatInt(liabilityTotal, 10)})
	out = append(out, []string{"total_equity", "", "", "", strconv.FormatInt(equityTotal, 10)})
	return writeCSVFile(zw, "balance-sheet.csv", out)
}

func (hc *HandlerContext) writeTrialBalanceCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.TrialBalance(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	out := [][]string{{"account_id", "account_name", "account_type", "debit_cents", "credit_cents"}}
	var totalDebit, totalCredit int64
	for _, row := range rows {
		net := row.DebitCents - row.CreditCents
		if net == 0 {
			continue
		}
		debit, credit := reporting.SplitDebitCredit(net)
		totalDebit += debit
		totalCredit += credit
		out = append(out, []string{row.AccountID, row.AccountName, row.AccountType, strconv.FormatInt(debit, 10), strconv.FormatInt(credit, 10)})
	}
	out = append(out, []string{"total", "", "", strconv.FormatInt(totalDebit, 10), strconv.FormatInt(totalCredit, 10)})
	return writeCSVFile(zw, "trial-balance.csv", out)
}

func (hc *HandlerContext) writeVendorPaymentsCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.VendorPayments(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	const threshold int64 = 60000 // $600 — the 1099-NEC reporting floor
	out := [][]string{{"vendor_name", "tax_id", "total_cents", "needs_1099"}}
	for _, row := range rows {
		needs := "false"
		if row.TotalCents >= threshold {
			needs = "true"
		}
		out = append(out, []string{row.VendorName, row.TaxID, strconv.FormatInt(row.TotalCents, 10), needs})
	}
	return writeCSVFile(zw, "vendor-payments-1099.csv", out)
}

func (hc *HandlerContext) writeGeneralLedgerCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.GeneralLedger(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	out := [][]string{transactionExportHeader()}
	for _, row := range rows {
		out = append(out, transactionExportRow(row))
	}
	return writeCSVFile(zw, "general-ledger.csv", out)
}

func (hc *HandlerContext) writeTransactionsCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	rows, err := hc.reports.TransactionsForExport(c.Request.Context(), entityID, start, end)
	if err != nil {
		return err
	}
	out := [][]string{transactionExportHeader()}
	for _, row := range rows {
		out = append(out, transactionExportRow(row))
	}
	return writeCSVFile(zw, "transactions.csv", out)
}

func (hc *HandlerContext) writeMileageCSV(c *gin.Context, zw *zip.Writer, entityID string, start, end time.Time) error {
	out := [][]string{{"date", "distance_miles", "start_location", "end_location", "purpose", "receipt_id", "user_id"}}
	if hc.mileage == nil {
		return writeCSVFile(zw, "mileage.csv", out)
	}
	logs, err := hc.mileage.Export(c.Request.Context(), entityID, &start, &end)
	if err != nil {
		return err
	}
	for _, log := range logs {
		out = append(out, []string{
			log.Date.Format("2006-01-02"),
			strconv.FormatFloat(log.DistanceMiles, 'f', 3, 64),
			log.StartLocation,
			log.EndLocation,
			log.Purpose,
			log.ReceiptID,
			log.UserID,
		})
	}
	return writeCSVFile(zw, "mileage.csv", out)
}

func (hc *HandlerContext) taxImportDiagnostics(c *gin.Context, entityID string, start, end time.Time) ([]taxException, [][]string, error) {
	exceptions := []taxException{}
	summary := [][]string{{"import_id", "file", "status", "row_count", "parsed_rows", "error_rows", "outflow_cents", "inflow_cents", "posted_outflow_cents", "posted_inflow_cents", "mapped_rows", "posted_rows", "unposted_rows", "duplicate_rows"}}
	if hc.receiptStore == nil {
		return exceptions, summary, nil
	}
	receipts, err := hc.receiptStore.List(c.Request.Context(), entityID, "", 10000)
	if err != nil {
		return nil, nil, err
	}
	duplicateRefs := map[string][]taxException{}
	for _, receipt := range receipts {
		if receipt.Kind == "import" && hc.importRows != nil {
			rows, err := hc.importRows.ListByReceiptID(c.Request.Context(), receipt.ID)
			if err == nil && len(rows) > 0 {
				rows = importRowsInScope(rows, start, end)
				if len(rows) == 0 {
					continue
				}
				summary = append(summary, importRowsSummary(receipt.ID, receipt.OriginalName, receipt.Status, rows))
				for _, row := range rows {
					if row.Fingerprint != "" {
						duplicateRefs[row.Fingerprint] = append(duplicateRefs[row.Fingerprint], taxException{
							SourceID:   receipt.ID,
							SourceName: receipt.OriginalName,
							Kind:       "import_row",
							Status:     row.Status,
							Issue:      "duplicate import row fingerprint",
							RowIndex:   strconv.Itoa(row.RowIndex),
							Vendor:     row.Vendor,
							Amount:     strconv.FormatInt(row.AmountCents, 10),
						})
					}
					if row.AccountID == "" {
						exceptions = append(exceptions, taxException{
							SourceID:   receipt.ID,
							SourceName: receipt.OriginalName,
							Kind:       "import_row",
							Status:     row.Status,
							Issue:      "unmapped import row",
							RowIndex:   strconv.Itoa(row.RowIndex),
							Vendor:     row.Vendor,
							Amount:     strconv.FormatInt(row.AmountCents, 10),
						})
						continue
					}
					if row.Status != "posted" {
						exceptions = append(exceptions, taxException{
							SourceID:   receipt.ID,
							SourceName: receipt.OriginalName,
							Kind:       "import_row",
							Status:     row.Status,
							Issue:      "import row not posted",
							RowIndex:   strconv.Itoa(row.RowIndex),
							Vendor:     row.Vendor,
							Amount:     strconv.FormatInt(row.AmountCents, 10),
						})
					}
				}
				continue
			}
		}
		if !timeInTaxScope(receipt.UploadedAt, start, end) {
			continue
		}
		if receipt.Status != "posted" {
			exceptions = append(exceptions, taxException{
				SourceID:   receipt.ID,
				SourceName: receipt.OriginalName,
				Kind:       receipt.Kind,
				Status:     receipt.Status,
				Issue:      "not posted",
			})
		}
		if receipt.Kind != "import" || hc.suggestions == nil {
			continue
		}
		suggestion, err := hc.suggestions.LatestByReceiptID(c.Request.Context(), receipt.ID)
		if err != nil {
			summary = append(summary, []string{receipt.ID, receipt.OriginalName, receipt.Status, "", "", "", "", "", "", "", "", "", "", ""})
			exceptions = append(exceptions, taxException{
				SourceID:   receipt.ID,
				SourceName: receipt.OriginalName,
				Kind:       receipt.Kind,
				Status:     receipt.Status,
				Issue:      "missing import suggestion/row mapping",
			})
			continue
		}
		importSummary, importRows, importErrors := parseImportSuggestionPayload(suggestion.ParsedJSON)
		summary = append(summary, []string{
			receipt.ID,
			receipt.OriginalName,
			receipt.Status,
			stringValue(importSummary, "row_count"),
			stringValue(importSummary, "parsed_rows"),
			stringValue(importSummary, "error_rows"),
			stringValue(importSummary, "outflow_cents"),
			stringValue(importSummary, "inflow_cents"),
			"",
			"",
			"",
			"",
			"",
			stringValue(importSummary, "duplicate_rows"),
		})
		for _, row := range importRows {
			if !mapDateInTaxScope(row, start, end) {
				continue
			}
			if stringValue(row, "account_id") == "" {
				exceptions = append(exceptions, taxException{
					SourceID:   receipt.ID,
					SourceName: receipt.OriginalName,
					Kind:       "import_row",
					Status:     receipt.Status,
					Issue:      "unmapped import row",
					RowIndex:   stringValue(row, "row_index"),
					Vendor:     stringValue(row, "vendor"),
					Amount:     stringValue(row, "amount_cents"),
				})
			}
		}
		for _, row := range importErrors {
			if !mapDateInTaxScope(row, start, end) {
				continue
			}
			exceptions = append(exceptions, taxException{
				SourceID:   receipt.ID,
				SourceName: receipt.OriginalName,
				Kind:       "import_row",
				Status:     receipt.Status,
				Issue:      "parse error: " + stringValue(row, "error"),
				RowIndex:   stringValue(row, "row_index"),
			})
		}
	}
	for _, refs := range duplicateRefs {
		if len(refs) > 1 {
			exceptions = append(exceptions, refs...)
		}
	}
	return exceptions, summary, nil
}

func importRowsInScope(rows []models.ImportRow, start, end time.Time) []models.ImportRow {
	out := make([]models.ImportRow, 0, len(rows))
	for _, row := range rows {
		if timeInTaxScope(row.Date, start, end) {
			out = append(out, row)
		}
	}
	return out
}

func timeInTaxScope(value, start, end time.Time) bool {
	if value.IsZero() {
		return true
	}
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	return !date.Before(start) && !date.After(end)
}

func mapDateInTaxScope(values map[string]interface{}, start, end time.Time) bool {
	raw := strings.TrimSpace(stringValue(values, "date"))
	if raw == "" {
		return true
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return true
	}
	return timeInTaxScope(value, start, end)
}

func importRowsSummary(receiptID, fileName, status string, rows []models.ImportRow) []string {
	var outflow, inflow, postedOutflow, postedInflow int64
	var mappedRows, postedRows, unpostedRows int
	duplicates := map[string]int{}
	duplicateIndexes := []string{}
	for _, row := range rows {
		if row.Direction == "inflow" {
			inflow += row.AmountCents
			if row.Status == "posted" {
				postedInflow += row.AmountCents
			}
		} else {
			outflow += row.AmountCents
			if row.Status == "posted" {
				postedOutflow += row.AmountCents
			}
		}
		if row.AccountID != "" {
			mappedRows++
		}
		if row.Status == "posted" {
			postedRows++
		} else {
			unpostedRows++
		}
		duplicates[row.Fingerprint]++
		if duplicates[row.Fingerprint] == 2 {
			duplicateIndexes = append(duplicateIndexes, strconv.Itoa(row.RowIndex))
		}
	}
	return []string{
		receiptID,
		fileName,
		status,
		strconv.Itoa(len(rows)),
		strconv.Itoa(len(rows)),
		"0",
		strconv.FormatInt(outflow, 10),
		strconv.FormatInt(inflow, 10),
		strconv.FormatInt(postedOutflow, 10),
		strconv.FormatInt(postedInflow, 10),
		strconv.Itoa(mappedRows),
		strconv.Itoa(postedRows),
		strconv.Itoa(unpostedRows),
		strings.Join(duplicateIndexes, " "),
	}
}

func (hc *HandlerContext) taxStatementExceptions(c *gin.Context, entityID string, start, end time.Time) []taxException {
	if hc.accountStatements == nil {
		return nil
	}
	statements, err := hc.accountStatements.List(c.Request.Context(), entityID, "", &start, &end, 1000)
	if err != nil {
		return nil
	}
	out := []taxException{}
	for _, statement := range statements {
		source := statement.AccountName
		if statement.SourceReceiptName != "" {
			source = statement.SourceReceiptName
		}
		base := taxException{
			SourceID:   statement.ID,
			SourceName: source,
			Kind:       "account_statement",
			Status:     statement.Status,
		}
		if statement.DifferenceCents != 0 {
			item := base
			item.Issue = "statement balance difference"
			item.Amount = strconv.FormatInt(statement.DifferenceCents, 10)
			out = append(out, item)
		}
		if statement.UnpostedRows > 0 {
			item := base
			item.Issue = "statement import rows not posted"
			item.Amount = strconv.Itoa(statement.UnpostedRows)
			out = append(out, item)
		}
		if statement.Status != "reconciled" && statement.Status != "locked" {
			item := base
			item.Issue = "statement not reconciled"
			out = append(out, item)
		}
	}
	return out
}

func (hc *HandlerContext) taxStatementRows(c *gin.Context, entityID string, start, end time.Time) [][]string {
	rows := [][]string{{
		"statement_id",
		"account_id",
		"account_name",
		"source_import_id",
		"source_import_name",
		"period_start",
		"period_end",
		"starting_balance_cents",
		"ending_balance_cents",
		"imported_inflow_cents",
		"imported_outflow_cents",
		"posted_inflow_cents",
		"posted_outflow_cents",
		"expected_ending_balance_cents",
		"difference_cents",
		"unposted_rows",
		"status",
	}}
	if hc.accountStatements == nil {
		return rows
	}
	statements, err := hc.accountStatements.List(c.Request.Context(), entityID, "", &start, &end, 1000)
	if err != nil {
		return rows
	}
	for _, statement := range statements {
		rows = append(rows, []string{
			statement.ID,
			statement.AccountID,
			statement.AccountName,
			statement.SourceReceiptID,
			statement.SourceReceiptName,
			statement.PeriodStart.Format("2006-01-02"),
			statement.PeriodEnd.Format("2006-01-02"),
			strconv.FormatInt(statement.StartingBalanceCents, 10),
			strconv.FormatInt(statement.EndingBalanceCents, 10),
			strconv.FormatInt(statement.ImportedInflowCents, 10),
			strconv.FormatInt(statement.ImportedOutflowCents, 10),
			strconv.FormatInt(statement.PostedInflowCents, 10),
			strconv.FormatInt(statement.PostedOutflowCents, 10),
			strconv.FormatInt(statement.ExpectedEndingBalanceCents, 10),
			strconv.FormatInt(statement.DifferenceCents, 10),
			strconv.Itoa(statement.UnpostedRows),
			statement.Status,
		})
	}
	return rows
}

func taxActionItems(exceptions []taxException) []taxActionItem {
	type actionKey struct {
		kind     string
		sourceID string
	}
	grouped := map[actionKey]taxActionItem{}
	order := []actionKey{}
	for _, item := range exceptions {
		action := taxActionForException(item)
		key := actionKey{kind: action.Kind, sourceID: item.SourceID}
		if existing, ok := grouped[key]; ok {
			existing.Count++
			grouped[key] = existing
			continue
		}
		action.Count = 1
		grouped[key] = action
		order = append(order, key)
	}
	out := make([]taxActionItem, 0, len(order))
	for _, key := range order {
		out = append(out, grouped[key])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

func taxActionForException(item taxException) taxActionItem {
	source := item.SourceName
	if source == "" {
		source = item.SourceID
	}
	action := taxActionItem{
		Kind:     "review",
		Label:    "Review " + source,
		Href:     "/imports/" + item.SourceID,
		Priority: 50,
	}
	switch {
	case item.Issue == "unmapped import row":
		action.Kind = "map_import_rows"
		action.Label = "Map uncategorized rows in " + source
		action.Priority = 10
	case item.Issue == "import row not posted":
		action.Kind = "post_import_rows"
		action.Label = "Post mapped rows in " + source
		action.Priority = 20
	case item.Issue == "duplicate import row fingerprint":
		action.Kind = "review_duplicates"
		action.Label = "Review duplicate-suspect rows in " + source
		action.Priority = 30
	case strings.HasPrefix(item.Issue, "parse error:"):
		action.Kind = "fix_parse_errors"
		action.Label = "Fix import parse errors in " + source
		action.Priority = 40
	case item.Issue == "missing import suggestion/row mapping":
		action.Kind = "reprocess_import"
		action.Label = "Reprocess import mapping for " + source
		action.Priority = 45
	case item.Issue == "missing home-use allocation" || item.Issue == "partial home-use allocation":
		action.Kind = "configure_home_use_allocation"
		action.Label = "Configure home-use allocation in entity settings"
		action.Href = "/settings/entity"
		action.Priority = 5
	case item.Kind == "account_statement":
		action.Kind = "reconcile_statement"
		action.Label = "Reconcile statement " + source
		action.Href = "/statements"
		action.Priority = 15
	case item.Issue == "not posted":
		action.Kind = "post_receipt"
		action.Label = "Post receipt " + source
		action.Priority = 60
	}
	if item.Kind != "import_row" && item.SourceID != "" {
		action.Href = "/receipts/" + item.SourceID
	}
	if item.Kind == "entity" {
		action.Href = "/settings/entity"
	}
	if item.Kind == "account_statement" {
		action.Href = "/statements"
	}
	return action
}

func (hc *HandlerContext) taxUseProfileExceptions(c *gin.Context, entityID string, taxYear int) []taxException {
	profile := hc.loadTaxUseProfile(c, entityID, taxYear)
	if profile.Status == "complete" {
		return nil
	}
	issue := "missing home-use allocation"
	if profile.Status == "partial" {
		issue = "partial home-use allocation"
	}
	return []taxException{{
		SourceID:   entityID,
		SourceName: "Entity settings",
		Kind:       "entity",
		Status:     profile.Status,
		Issue:      issue,
	}}
}

func parseImportSuggestionPayload(raw json.RawMessage) (map[string]interface{}, []map[string]interface{}, []map[string]interface{}) {
	payload := map[string]interface{}{}
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil, nil, nil
	}
	summary, _ := payload["import_summary"].(map[string]interface{})
	rows := objectSlice(payload["import_rows"])
	errors := objectSlice(payload["import_errors"])
	return summary, rows, errors
}

func objectSlice(raw interface{}) []map[string]interface{} {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]interface{}); ok {
			out = append(out, row)
		}
	}
	return out
}

func stringValue(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	v, ok := values[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprint(val)
	}
}

func taxExceptionsRows(exceptions []taxException) [][]string {
	rows := [][]string{{"source_id", "source_name", "kind", "status", "issue", "row_index", "vendor", "amount_cents", "href"}}
	for _, item := range exceptions {
		rows = append(rows, []string{item.SourceID, item.SourceName, item.Kind, item.Status, item.Issue, item.RowIndex, item.Vendor, item.Amount, taxExceptionHref(item)})
	}
	return rows
}

func taxActionRows(actions []taxActionItem) [][]string {
	rows := [][]string{{"kind", "label", "count", "href", "priority"}}
	for _, item := range actions {
		rows = append(rows, []string{
			item.Kind,
			item.Label,
			strconv.Itoa(item.Count),
			item.Href,
			strconv.Itoa(item.Priority),
		})
	}
	return rows
}

func blockingSummaryRows(exceptions []taxException) []gin.H {
	grouped := map[string]*taxBlockingSummary{}
	order := []string{}
	for _, item := range exceptions {
		key := item.SourceID
		if key == "" {
			key = item.SourceName
		}
		summary, ok := grouped[key]
		if !ok {
			summary = &taxBlockingSummary{
				SourceID:   item.SourceID,
				SourceName: item.SourceName,
				Kind:       item.Kind,
				Status:     item.Status,
			}
			grouped[key] = summary
			order = append(order, key)
		}
		summary.IssueCount++
		if summary.SourceName == "" {
			summary.SourceName = item.SourceName
		}
		if summary.Status == "" {
			summary.Status = item.Status
		}
		if summary.Kind != "receipt" && item.Kind == "receipt" {
			summary.Kind = "receipt"
		}
		if item.RowIndex != "" && summary.FirstRowIndex == "" {
			summary.FirstRowIndex = item.RowIndex
		}
		if item.RowIndex != "" {
			summary.Href = taxExceptionHref(item)
		} else if summary.Href == "" {
			summary.Href = taxExceptionHref(item)
		}
		switch {
		case item.Issue == "unmapped import row":
			summary.UnmappedRows++
		case item.Issue == "duplicate import row fingerprint":
			summary.DuplicateRows++
		case item.Issue == "import row not posted" || item.Issue == "not posted":
			summary.NotPosted++
		case strings.HasPrefix(item.Issue, "parse error:"):
			summary.ParseErrors++
		}
		if summary.Href == "" {
			summary.Href = taxExceptionHref(item)
		}
	}
	summaries := make([]taxBlockingSummary, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, *grouped[key])
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IssueCount == summaries[j].IssueCount {
			return summaries[i].SourceName < summaries[j].SourceName
		}
		return summaries[i].IssueCount > summaries[j].IssueCount
	})
	rows := make([]gin.H, 0, len(summaries))
	for _, item := range summaries {
		rows = append(rows, gin.H{
			"source_id":       item.SourceID,
			"source_name":     item.SourceName,
			"kind":            item.Kind,
			"status":          item.Status,
			"issue_count":     item.IssueCount,
			"unmapped_rows":   item.UnmappedRows,
			"duplicate_rows":  item.DuplicateRows,
			"not_posted":      item.NotPosted,
			"parse_errors":    item.ParseErrors,
			"first_row_index": item.FirstRowIndex,
			"href":            item.Href,
		})
	}
	return rows
}

func (hc *HandlerContext) taxUseProfileResponse(c *gin.Context, entityID string, taxYear int) gin.H {
	profile := hc.loadTaxUseProfile(c, entityID, taxYear)
	return gin.H{
		"tax_year":                            profile.TaxYear,
		"status":                              profile.Status,
		"home_office_sqft":                    profile.HomeOfficeSqFt,
		"home_total_sqft":                     profile.HomeTotalSqFt,
		"home_utilities_business_use_percent": profile.HomeUtilitiesBusinessUsePercent,
		"cell_phone_business_use_percent":     profile.CellPhoneBusinessUsePercent,
		"home_internet_business_use_percent":  profile.HomeInternetBusinessUsePercent,
		"href":                                profile.Href,
	}
}

func (hc *HandlerContext) loadTaxUseProfile(c *gin.Context, entityID string, taxYear int) taxUseProfileState {
	profile := taxUseProfileState{
		TaxYear: taxYear,
		Status:  "missing",
		Href:    "/settings/entity",
	}
	if hc == nil || hc.entityTaxSettings == nil {
		return profile
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	settings, err := hc.entityTaxSettings.Get(ctx, entityID, taxYear)
	if err != nil {
		return profile
	}
	profile.HomeOfficeSqFt = intPtr(settings.HomeOfficeSqFt)
	profile.HomeTotalSqFt = intPtr(settings.HomeTotalSqFt)
	profile.CellPhoneBusinessUsePercent = intPtr(settings.CellPhoneBusinessUsePercent)
	profile.HomeInternetBusinessUsePercent = intPtr(settings.HomeInternetBusinessUsePercent)
	if percent, ok := db.UtilitiesBusinessUsePercent(settings.HomeOfficeSqFt, settings.HomeTotalSqFt); ok {
		profile.HomeUtilitiesBusinessUsePercent = &percent
	}
	switch {
	case settings.CreatedAt.IsZero():
		profile.Status = "missing"
	case settings.HomeOfficeSqFt.Valid && settings.HomeTotalSqFt.Valid && settings.CellPhoneBusinessUsePercent.Valid && settings.HomeInternetBusinessUsePercent.Valid:
		profile.Status = "complete"
	default:
		profile.Status = "partial"
	}
	return profile
}

func (hc *HandlerContext) taxUseAllocationRows(c *gin.Context, entityID string, taxYear int) [][]string {
	profile := hc.loadTaxUseProfile(c, entityID, taxYear)
	rows := [][]string{{"tax_year", "item", "status", "value", "details", "href"}}
	rows = append(rows, taxUseAllocationRow(taxYear, "utilities", "Home utilities allocation", profile.HomeUtilitiesBusinessUsePercent, profile.Status, profile.Href, profile.HomeOfficeSqFt, profile.HomeTotalSqFt))
	rows = append(rows, taxUseAllocationRow(taxYear, "cell_phone", "Cell phone allocation", profile.CellPhoneBusinessUsePercent, profile.Status, profile.Href, nil, nil))
	rows = append(rows, taxUseAllocationRow(taxYear, "internet", "Home internet allocation", profile.HomeInternetBusinessUsePercent, profile.Status, profile.Href, nil, nil))
	return rows
}

func taxUseAllocationRow(taxYear int, section, item string, percent *int, status, href string, homeOfficeSqFt, homeTotalSqFt *int) []string {
	rowStatus := "ready"
	value := ""
	details := "Not configured for this tax year."
	if percent == nil {
		rowStatus = "needs attention"
	} else {
		value = strconv.Itoa(*percent) + "%"
		details = "Business-use percentage recorded for the selected tax year."
	}
	if section == "utilities" {
		if homeOfficeSqFt != nil && homeTotalSqFt != nil {
			details = fmt.Sprintf("%d / %d sqft of the home used for business.", *homeOfficeSqFt, *homeTotalSqFt)
		} else if percent != nil {
			details = "Utilities allocation is derived from the stored home-office square-foot ratio."
		}
	}
	if status != "complete" {
		rowStatus = "needs attention"
	}
	return []string{strconv.Itoa(taxYear), item, rowStatus, value, details, href}
}

func taxBlockingSummaryRows(exceptions []taxException) [][]string {
	rows := [][]string{{"source_id", "source_name", "kind", "status", "issue_count", "unmapped_rows", "duplicate_rows", "not_posted", "parse_errors", "first_row_index", "href"}}
	for _, item := range blockingSummaryRows(exceptions) {
		rows = append(rows, []string{
			stringValue(item, "source_id"),
			stringValue(item, "source_name"),
			stringValue(item, "kind"),
			stringValue(item, "status"),
			stringValue(item, "issue_count"),
			stringValue(item, "unmapped_rows"),
			stringValue(item, "duplicate_rows"),
			stringValue(item, "not_posted"),
			stringValue(item, "parse_errors"),
			stringValue(item, "first_row_index"),
			stringValue(item, "href"),
		})
	}
	return rows
}

// allocationRow is one home-office allocation line. Both the tax-readiness
// checklist and the prepared-package checklist render these three rows (the
// former prefixes an "allocations" section column, the latter does not).
type allocationRow struct {
	label   string
	ok      bool
	count   int
	details string
}

func allocationChecklist(profile taxUseProfileState) []allocationRow {
	return []allocationRow{
		{"Home utilities allocation", profile.HomeUtilitiesBusinessUsePercent != nil, allocateCount(profile.HomeUtilitiesBusinessUsePercent != nil), utilitiesDetails(profile)},
		{"Cell phone allocation", profile.CellPhoneBusinessUsePercent != nil, allocateCount(profile.CellPhoneBusinessUsePercent != nil), percentageDetails("Cell phone", profile.CellPhoneBusinessUsePercent)},
		{"Home internet allocation", profile.HomeInternetBusinessUsePercent != nil, allocateCount(profile.HomeInternetBusinessUsePercent != nil), percentageDetails("Home internet", profile.HomeInternetBusinessUsePercent)},
	}
}

func (hc *HandlerContext) taxPrepChecklistRows(c *gin.Context, entityID string, start, end time.Time, exceptions []taxException) [][]string {
	rows := [][]string{{"section", "item", "status", "count", "details", "href"}}
	taxYear := start.Year()
	profile := hc.loadTaxUseProfile(c, entityID, taxYear)
	var mileageRows []gin.H
	if hc != nil && hc.mileage != nil {
		if rowsByMonth, err := hc.mileage.SummaryByMonth(c.Request.Context(), entityID, start, end, hc.mileageRates); err == nil {
			for _, row := range rowsByMonth {
				item := gin.H{
					"month":       row.Month.Format("2006-01"),
					"total_miles": row.TotalMiles,
					"trip_count":  row.TripCount,
				}
				if row.HasRate {
					item["rate_cents_per_mile"] = row.RateCents
					item["reimbursed_cents"] = row.ReimbCents
				} else {
					item["rate_cents_per_mile"] = nil
					item["reimbursed_cents"] = nil
					item["rate_missing"] = true
				}
				mileageRows = append(mileageRows, item)
			}
		}
	}

	var unmappedRows, duplicateRows, notPostedRows, parseErrors, receiptMissing, mileageMissingRate, mileageTrips int
	for _, item := range exceptions {
		switch {
		case item.Issue == "unmapped import row":
			unmappedRows++
		case item.Issue == "duplicate import row fingerprint":
			duplicateRows++
		case item.Issue == "import row not posted":
			notPostedRows++
		case strings.HasPrefix(item.Issue, "parse error:"):
			parseErrors++
		case item.Issue == "not posted" && item.Kind == "receipt":
			receiptMissing++
		}
	}
	for _, row := range mileageRows {
		mileageTrips += int(floatValue(row, "trip_count"))
		if boolValue(row, "rate_missing") {
			mileageMissingRate++
		}
	}
	for _, a := range allocationChecklist(profile) {
		rows = append(rows, []string{"allocations", a.label, checklistStatus(a.ok), strconv.Itoa(a.count), a.details, "/settings/entity"})
	}
	coverage := hc.loadAccountRoleCoverage(c, entityID)
	rows = append(rows, []string{
		"allocations",
		"Utilities tagged accounts",
		checklistStatus(profile.HomeUtilitiesBusinessUsePercent == nil || coverage.UtilitiesCount > 0),
		strconv.Itoa(coverage.UtilitiesCount),
		"Accounts tagged as utilities for tax allocation.",
		"/accounts",
	})
	rows = append(rows, []string{
		"allocations",
		"Cell phone tagged accounts",
		checklistStatus(profile.CellPhoneBusinessUsePercent == nil || coverage.CellPhoneCount > 0),
		strconv.Itoa(coverage.CellPhoneCount),
		"Accounts tagged as cell phone for tax allocation.",
		"/accounts",
	})
	rows = append(rows, []string{
		"allocations",
		"Internet tagged accounts",
		checklistStatus(profile.HomeInternetBusinessUsePercent == nil || coverage.InternetCount > 0),
		strconv.Itoa(coverage.InternetCount),
		"Accounts tagged as internet for tax allocation.",
		"/accounts",
	})
	rows = append(rows, []string{
		"imports",
		"Unmapped rows",
		checklistStatus(unmappedRows == 0),
		strconv.Itoa(unmappedRows),
		"Import rows without a mapped account.",
		"/review",
	})
	rows = append(rows, []string{
		"imports",
		"Duplicate-suspect rows",
		checklistStatus(duplicateRows == 0),
		strconv.Itoa(duplicateRows),
		"Rows with repeated fingerprints that deserve a manual pass.",
		"/exports",
	})
	rows = append(rows, []string{
		"imports",
		"Unposted import rows",
		checklistStatus(notPostedRows == 0),
		strconv.Itoa(notPostedRows),
		"Mapped rows that have not been posted yet.",
		"/review",
	})
	rows = append(rows, []string{
		"imports",
		"Parse errors",
		checklistStatus(parseErrors == 0),
		strconv.Itoa(parseErrors),
		"Import rows that could not be parsed cleanly.",
		"/review",
	})
	rows = append(rows, []string{
		"receipts",
		"Unposted receipts",
		checklistStatus(receiptMissing == 0),
		strconv.Itoa(receiptMissing),
		"Receipts still waiting for posting.",
		"/receipts",
	})
	rows = append(rows, []string{
		"mileage",
		"Mileage trips",
		checklistStatus(true),
		strconv.Itoa(mileageTrips),
		"Trips found in the selected scope.",
		"/mileage",
	})
	rows = append(rows, []string{
		"mileage",
		"Missing mileage rates",
		checklistStatus(mileageMissingRate == 0),
		strconv.Itoa(mileageMissingRate),
		"Months in scope without a reimbursement rate.",
		"/mileage-rates",
	})
	return rows
}

func (hc *HandlerContext) taxPreparedPackageRows(c *gin.Context, entityID string, start, end time.Time, exceptions []taxException, importSummary [][]string) [][]string {
	rows := [][]string{{"item", "status", "count", "details", "href"}}
	blockers := blockingSummaryRows(exceptions)
	profile := hc.loadTaxUseProfile(c, entityID, start.Year())
	var mileageTrips int
	var mileageMissingRates int
	if hc != nil && hc.mileage != nil {
		if rowsByMonth, err := hc.mileage.SummaryByMonth(c.Request.Context(), entityID, start, end, hc.mileageRates); err == nil {
			for _, row := range rowsByMonth {
				mileageTrips += row.TripCount
				if !row.HasRate {
					mileageMissingRates++
				}
			}
		}
	}
	for _, a := range allocationChecklist(profile) {
		rows = append(rows, []string{a.label, checklistStatus(a.ok), strconv.Itoa(a.count), a.details, "/settings/entity"})
	}
	coverage := hc.loadAccountRoleCoverage(c, entityID)
	rows = append(rows, []string{
		"Tagged accounts",
		"Utilities",
		checklistStatus(profile.HomeUtilitiesBusinessUsePercent == nil || coverage.UtilitiesCount > 0),
		strconv.Itoa(coverage.UtilitiesCount),
		"Accounts tagged for utilities allocations.",
		"/accounts",
	})
	rows = append(rows, []string{
		"Tagged accounts",
		"Cell phone",
		checklistStatus(profile.CellPhoneBusinessUsePercent == nil || coverage.CellPhoneCount > 0),
		strconv.Itoa(coverage.CellPhoneCount),
		"Accounts tagged for cell phone allocations.",
		"/accounts",
	})
	rows = append(rows, []string{
		"Tagged accounts",
		"Internet",
		checklistStatus(profile.HomeInternetBusinessUsePercent == nil || coverage.InternetCount > 0),
		strconv.Itoa(coverage.InternetCount),
		"Accounts tagged for internet allocations.",
		"/accounts",
	})
	rows = append(rows, []string{
		"Tax pack",
		checklistStatus(len(exceptions) == 0 && mileageMissingRates == 0),
		strconv.Itoa(len(exceptions)),
		"Ready to hand off when blockers are clear.",
		"/exports",
	})
	rows = append(rows, []string{
		"Import sources",
		checklistStatus(len(importSummary) > 1),
		strconv.Itoa(max(0, len(importSummary)-1)),
		"Imports in the selected tax scope.",
		"/imports",
	})
	rows = append(rows, []string{
		"Open blockers",
		checklistStatus(len(blockers) == 0),
		strconv.Itoa(len(blockers)),
		"Sources that still need review.",
		"/review",
	})
	rows = append(rows, []string{
		"Unmapped rows",
		checklistStatus(sumBlockingCount(blockers, "unmapped_rows") == 0),
		strconv.Itoa(sumBlockingCount(blockers, "unmapped_rows")),
		"Rows still missing an account.",
		"/review",
	})
	rows = append(rows, []string{
		"Duplicate suspects",
		checklistStatus(sumBlockingCount(blockers, "duplicate_rows") == 0),
		strconv.Itoa(sumBlockingCount(blockers, "duplicate_rows")),
		"Rows that should be checked for repeated fingerprints.",
		"/imports",
	})
	rows = append(rows, []string{
		"Unposted rows",
		checklistStatus(sumBlockingCount(blockers, "not_posted") == 0),
		strconv.Itoa(sumBlockingCount(blockers, "not_posted")),
		"Mapped rows that still need posting.",
		"/review",
	})
	rows = append(rows, []string{
		"Mileage trips",
		checklistStatus(true),
		strconv.Itoa(mileageTrips),
		"Mileage logs in the selected filing scope.",
		"/mileage",
	})
	rows = append(rows, []string{
		"Mileage rate gaps",
		checklistStatus(mileageMissingRates == 0),
		strconv.Itoa(mileageMissingRates),
		"Months without a mileage reimbursement rate.",
		"/mileage-rates",
	})
	return rows
}

func sumBlockingCount(rows []gin.H, key string) int {
	total := 0
	for _, row := range rows {
		total += intValue(row, key)
	}
	return total
}

func intValue(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	v, ok := values[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func checklistStatus(ok bool) string {
	if ok {
		return "ready"
	}
	return "needs attention"
}

func allocateCount(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func utilitiesDetails(profile taxUseProfileState) string {
	if profile.HomeUtilitiesBusinessUsePercent == nil {
		return "Set home-office square feet and total home square feet for the tax year."
	}
	if profile.HomeOfficeSqFt != nil && profile.HomeTotalSqFt != nil {
		return fmt.Sprintf("%d%% based on %d of %d sqft.", *profile.HomeUtilitiesBusinessUsePercent, *profile.HomeOfficeSqFt, *profile.HomeTotalSqFt)
	}
	return fmt.Sprintf("%d%% utilities allocation.", *profile.HomeUtilitiesBusinessUsePercent)
}

func percentageDetails(label string, percent *int) string {
	if percent == nil {
		return label + " business-use percentage is not configured for this tax year."
	}
	return fmt.Sprintf("%d%% business use recorded for this tax year.", *percent)
}

func boolValue(values map[string]interface{}, key string) bool {
	if values == nil {
		return false
	}
	v, ok := values[key]
	if !ok || v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func floatValue(values map[string]interface{}, key string) float64 {
	if values == nil {
		return 0
	}
	v, ok := values[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func writeCSVFile(zw *zip.Writer, name string, rows [][]string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeTextFile(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}

func taxPackReadme(entityID string, start, end time.Time, label string, exceptionCount int) string {
	return fmt.Sprintf(`# Open B00KS Tax Pack

Entity ID: %s
Scope: %s through %s
Label: %s

Files:
- profit-loss.csv: income and expense totals by account.
- balance-sheet.csv: assets, liabilities, and equity with section totals.
- trial-balance.csv: every account as a debit or credit balance with column totals.
- general-ledger.csv: posted journal-entry lines.
- transactions.csv: same posted transaction-line export, provided for spreadsheet workflows.
- mileage.csv: mileage logs in the selected scope.
- vendor-payments-1099.csv: total expense paid per matched vendor, with 1099-NEC candidates flagged.
- import-summary.csv: import-level row counts, totals, and duplicate flags.
- review-actions.csv: grouped follow-up items derived from tax readiness exceptions.
- blocking-summary.csv: grouped issue counts by source with direct fix links.
- home-use-allocation.csv: entity home-office, cell phone, and internet allocation profile.
- account-role-tags.csv: tagged accounts used for utilities, cell phone, and internet allocation coverage.
- statement-reconciliation.csv: statement starting/ending balances, imported totals, posted totals, differences, and status.
- prep-checklist.csv: filing checklist with import, receipt, and mileage readiness items.
- exceptions.csv: unposted items, unmapped rows, parse errors, and missing import mappings.

Review checklist:
- exceptions.csv should be empty, or every item should be explained before filing.
- Reconcile import-summary.csv totals to bank and card statements.
- Reconcile statement-reconciliation.csv differences and unposted rows before filing.
- Confirm expense categories match your tax treatment.
- Review vendor-payments-1099.csv: confirm payment method (cash/check) and contractor status for flagged vendors.
- Confirm income, owner draws/contributions, assets, loans, and payroll outside this export when applicable.

Exception count: %d
`, entityID, start.Format("2006-01-02"), end.Format("2006-01-02"), label, exceptionCount)
}
