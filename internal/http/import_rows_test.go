package httpapi

import (
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestImportRowEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		row         models.ImportRow
		wantDebit   string
		wantCredit  string
		wantEntries int
	}{
		"outflow debits mapped account and credits cash": {
			row: models.ImportRow{
				Date:        time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				AccountID:   "expense-account",
				AmountCents: 1234,
				Direction:   "outflow",
			},
			wantDebit:   "expense-account",
			wantCredit:  "cash-account",
			wantEntries: 2,
		},
		"inflow debits cash and credits mapped account": {
			row: models.ImportRow{
				Date:        time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				AccountID:   "income-account",
				AmountCents: 5000,
				Direction:   "inflow",
			},
			wantDebit:   "cash-account",
			wantCredit:  "income-account",
			wantEntries: 2,
		},
		"missing account is invalid": {
			row: models.ImportRow{
				AmountCents: 5000,
				Direction:   "outflow",
			},
			wantEntries: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := importRowEntries(tt.row, "cash-account")
			if len(got) != tt.wantEntries {
				t.Fatalf("entries = %d, want %d", len(got), tt.wantEntries)
			}
			if tt.wantEntries == 0 {
				return
			}
			if got[0].AccountID != tt.wantDebit || got[0].DebitCents != tt.row.AmountCents {
				t.Fatalf("debit entry = %#v, want account %q amount %d", got[0], tt.wantDebit, tt.row.AmountCents)
			}
			if got[1].AccountID != tt.wantCredit || got[1].CreditCents != tt.row.AmountCents {
				t.Fatalf("credit entry = %#v, want account %q amount %d", got[1], tt.wantCredit, tt.row.AmountCents)
			}
		})
	}
}

func TestImportRowsSummaryReconcilesPostedTotals(t *testing.T) {
	t.Parallel()

	rows := []models.ImportRow{
		{
			RowIndex:    1,
			AmountCents: 1000,
			Direction:   "outflow",
			AccountID:   "expense",
			Status:      "posted",
			Fingerprint: "a",
		},
		{
			RowIndex:    2,
			AmountCents: 2500,
			Direction:   "outflow",
			AccountID:   "expense",
			Status:      "mapped",
			Fingerprint: "b",
		},
		{
			RowIndex:    3,
			AmountCents: 5000,
			Direction:   "inflow",
			AccountID:   "income",
			Status:      "posted",
			Fingerprint: "a",
		},
	}

	got := importRowsSummary("receipt", "bank.csv", "ready_for_review", rows)
	assertColumn := func(index int, want string) {
		t.Helper()
		if got[index] != want {
			t.Fatalf("summary[%d] = %q, want %q; row=%#v", index, got[index], want, got)
		}
	}

	assertColumn(3, "3")
	assertColumn(6, "3500")
	assertColumn(7, "5000")
	assertColumn(8, "1000")
	assertColumn(9, "5000")
	assertColumn(10, "3")
	assertColumn(11, "2")
	assertColumn(12, "1")
	assertColumn(13, "3")
}
