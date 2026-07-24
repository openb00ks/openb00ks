package importer

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type Direction string

const (
	DirectionOutflow Direction = "outflow"
	DirectionInflow  Direction = "inflow"
	DirectionUnknown Direction = "unknown"
)

type Row struct {
	RowIndex    int       `json:"row_index"`
	Date        string    `json:"date,omitempty"`
	Vendor      string    `json:"vendor,omitempty"`
	Memo        string    `json:"memo,omitempty"`
	AmountCents int64     `json:"amount_cents,omitempty"`
	Direction   Direction `json:"direction,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Raw         []string  `json:"raw,omitempty"`
}

type RowError struct {
	RowIndex int      `json:"row_index"`
	Field    string   `json:"field,omitempty"`
	Error    string   `json:"error"`
	Raw      []string `json:"raw,omitempty"`
}

type Summary struct {
	RowCount      int                      `json:"row_count"`
	ParsedRows    int                      `json:"parsed_rows"`
	ErrorRows     int                      `json:"error_rows"`
	OutflowCents  int64                    `json:"outflow_cents"`
	InflowCents   int64                    `json:"inflow_cents"`
	TotalCents    int64                    `json:"total_cents"`
	TopVendor     string                   `json:"top_vendor,omitempty"`
	TopVendors    []map[string]interface{} `json:"top_vendors,omitempty"`
	DuplicateRows []int                    `json:"duplicate_rows,omitempty"`
}

type Result struct {
	Rows    []Row      `json:"rows"`
	Errors  []RowError `json:"errors,omitempty"`
	Summary Summary    `json:"summary"`
}

type columns struct {
	date        int
	vendor      int
	memo        int
	amount      int
	debit       int
	credit      int
	description int
}

func ParseCSV(raw string) Result {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Result{}
	}

	reader := csv.NewReader(strings.NewReader(trimmed))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return Result{Errors: []RowError{{Error: err.Error()}}}
	}
	return parseRecords(records)
}

func ParseCSVReader(r io.Reader) Result {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return Result{Errors: []RowError{{Error: err.Error()}}}
	}
	return parseRecords(records)
}

func parseRecords(records [][]string) Result {
	if len(records) == 0 {
		return Result{}
	}

	cols := detectColumns(records[0])
	start := 0
	if cols.hasUsefulHeader() {
		start = 1
	} else {
		cols = columns{date: 0, vendor: 1, amount: 2, memo: -1, debit: -1, credit: -1, description: -1}
	}

	result := Result{
		Rows:   make([]Row, 0, len(records)-start),
		Errors: make([]RowError, 0),
	}
	seen := map[string]int{}
	vendors := map[string]struct {
		count int
		total int64
	}{}

	for i := start; i < len(records); i++ {
		raw := records[i]
		rowIndex := i - start + 1
		if isBlankRow(raw) {
			continue
		}
		result.Summary.RowCount++

		row, errs := parseRow(rowIndex, raw, cols)
		if len(errs) > 0 {
			result.Errors = append(result.Errors, errs...)
			result.Summary.ErrorRows++
			continue
		}
		if row.AmountCents <= 0 {
			result.Errors = append(result.Errors, RowError{
				RowIndex: rowIndex,
				Field:    "amount",
				Error:    "missing or zero amount",
				Raw:      raw,
			})
			result.Summary.ErrorRows++
			continue
		}
		row.Fingerprint = fingerprint(row.Date, row.Vendor, row.Memo, row.AmountCents, row.Direction)
		if previous, ok := seen[row.Fingerprint]; ok {
			result.Summary.DuplicateRows = append(result.Summary.DuplicateRows, previous, row.RowIndex)
		} else {
			seen[row.Fingerprint] = row.RowIndex
		}

		result.Rows = append(result.Rows, row)
		result.Summary.ParsedRows++
		switch row.Direction {
		case DirectionInflow:
			result.Summary.InflowCents += row.AmountCents
		default:
			result.Summary.OutflowCents += row.AmountCents
		}
		if row.Vendor != "" {
			agg := vendors[row.Vendor]
			agg.count++
			agg.total += row.AmountCents
			vendors[row.Vendor] = agg
		}
	}
	result.Summary.TotalCents = result.Summary.OutflowCents + result.Summary.InflowCents
	result.Summary.TopVendor, result.Summary.TopVendors = topVendors(vendors)
	result.Summary.DuplicateRows = uniqueInts(result.Summary.DuplicateRows)
	return result
}

func parseRow(rowIndex int, raw []string, cols columns) (Row, []RowError) {
	row := Row{
		RowIndex:  rowIndex,
		Direction: DirectionUnknown,
		Raw:       append([]string(nil), raw...),
	}
	errs := []RowError{}

	if cols.date >= 0 && cols.date < len(raw) {
		date, err := normalizeDate(raw[cols.date])
		if err != nil {
			errs = append(errs, RowError{RowIndex: rowIndex, Field: "date", Error: err.Error(), Raw: raw})
		} else {
			row.Date = date
		}
	}
	row.Vendor = firstNonEmpty(cell(raw, cols.vendor), cell(raw, cols.description))
	row.Memo = firstNonEmpty(cell(raw, cols.memo), cell(raw, cols.description), row.Vendor)
	if row.Vendor == "" {
		row.Vendor = row.Memo
	}

	amount, direction, err := parseAmount(raw, cols)
	if err != nil {
		errs = append(errs, RowError{RowIndex: rowIndex, Field: "amount", Error: err.Error(), Raw: raw})
	} else {
		row.AmountCents = amount
		row.Direction = direction
	}
	if row.Date == "" && cols.date >= 0 && strings.TrimSpace(cell(raw, cols.date)) == "" {
		errs = append(errs, RowError{RowIndex: rowIndex, Field: "date", Error: "missing date", Raw: raw})
	}
	if row.Vendor == "" {
		errs = append(errs, RowError{RowIndex: rowIndex, Field: "vendor", Error: "missing vendor or description", Raw: raw})
	}
	return row, errs
}

func detectColumns(header []string) columns {
	cols := columns{date: -1, vendor: -1, memo: -1, amount: -1, debit: -1, credit: -1, description: -1}
	for idx, cell := range header {
		norm := normalizeHeader(cell)
		switch {
		case containsAny(norm, "date", "posteddate", "transactiondate", "postingdate"):
			if cols.date == -1 {
				cols.date = idx
			}
		case containsAny(norm, "vendor", "merchant", "payee", "name"):
			if cols.vendor == -1 {
				cols.vendor = idx
			}
		case containsAny(norm, "description", "details", "memo", "narrative"):
			if cols.description == -1 {
				cols.description = idx
			}
			if cols.memo == -1 {
				cols.memo = idx
			}
		case norm == "amount" || strings.HasSuffix(norm, "amount") || strings.Contains(norm, "transactionamount"):
			if cols.amount == -1 {
				cols.amount = idx
			}
		case norm == "debit" || strings.Contains(norm, "withdrawal") || strings.Contains(norm, "charge"):
			if cols.debit == -1 {
				cols.debit = idx
			}
		case norm == "credit" || strings.Contains(norm, "deposit") || strings.Contains(norm, "payment"):
			if cols.credit == -1 {
				cols.credit = idx
			}
		}
	}
	return cols
}

func (c columns) hasUsefulHeader() bool {
	return c.date >= 0 || c.amount >= 0 || c.debit >= 0 || c.credit >= 0 || c.vendor >= 0 || c.description >= 0
}

func parseAmount(raw []string, cols columns) (int64, Direction, error) {
	if cols.debit >= 0 || cols.credit >= 0 {
		debit, hasDebit, err := parseMoneyCell(cell(raw, cols.debit))
		if err != nil {
			return 0, DirectionUnknown, err
		}
		credit, hasCredit, err := parseMoneyCell(cell(raw, cols.credit))
		if err != nil {
			return 0, DirectionUnknown, err
		}
		if hasDebit && debit != 0 {
			return abs64(debit), DirectionOutflow, nil
		}
		if hasCredit && credit != 0 {
			return abs64(credit), DirectionInflow, nil
		}
	}
	amount, ok, err := parseMoneyCell(cell(raw, cols.amount))
	if err != nil {
		return 0, DirectionUnknown, err
	}
	if !ok {
		return 0, DirectionUnknown, errors.New("missing amount")
	}
	if amount < 0 {
		return abs64(amount), DirectionOutflow, nil
	}
	return amount, DirectionInflow, nil
}

func parseMoneyCell(raw string) (int64, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, nil
	}
	negative := strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")")
	clean := strings.NewReplacer("$", "", ",", "", "(", "", ")", "", " ", "").Replace(trimmed)
	if clean == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid amount %q", raw)
	}
	if negative {
		value = -value
	}
	return int64(math.Round(value * 100)), true, nil
}

func normalizeDate(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("missing date")
	}
	layouts := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"01/02/06",
		"1/2/06",
		"Jan 2, 2006",
		"January 2, 2006",
		"2006/01/02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("invalid date %q", raw)
}

func fingerprint(date, vendor, memo string, amountCents int64, direction Direction) string {
	parts := []string{
		date,
		strings.ToLower(strings.Join(strings.Fields(vendor), " ")),
		strings.ToLower(strings.Join(strings.Fields(memo), " ")),
		strconv.FormatInt(amountCents, 10),
		string(direction),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func topVendors(vendors map[string]struct {
	count int
	total int64
}) (string, []map[string]interface{}) {
	type stat struct {
		name  string
		count int
		total int64
	}
	stats := make([]stat, 0, len(vendors))
	for name, agg := range vendors {
		stats = append(stats, stat{name: name, count: agg.count, total: agg.total})
	}
	for i := 0; i < len(stats); i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].total > stats[i].total || (stats[j].total == stats[i].total && stats[j].count > stats[i].count) || (stats[j].total == stats[i].total && stats[j].count == stats[i].count && stats[j].name < stats[i].name) {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}
	top := make([]map[string]interface{}, 0, 3)
	for i := 0; i < len(stats) && i < 3; i++ {
		top = append(top, map[string]interface{}{
			"vendor":      stats[i].name,
			"count":       stats[i].count,
			"total_cents": stats[i].total,
		})
	}
	if len(stats) == 0 {
		return "", top
	}
	return stats[0].name, top
}

func normalizeHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", ".", "", "/", "")
	return replacer.Replace(value)
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
