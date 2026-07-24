package httpapi

import (
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestPickExpenseAccount(t *testing.T) {
	types := map[string]string{
		"6200": "expense",
		"6800": "expense",
		"1000": "asset", // cash
		"2000": "liability",
	}

	tests := []struct {
		name    string
		entries []models.Entry
		want    string
	}{
		{
			name: "simple expense entry: the debited expense account",
			entries: []models.Entry{
				{AccountID: "6200", DebitCents: 1080},
				{AccountID: "1000", CreditCents: 1080},
			},
			want: "6200",
		},
		{
			name: "split expense: the largest expense debit wins",
			entries: []models.Entry{
				{AccountID: "6200", DebitCents: 300},
				{AccountID: "6800", DebitCents: 780},
				{AccountID: "1000", CreditCents: 1080},
			},
			want: "6800",
		},
		{
			name: "credited expense (a refund) is not a categorization choice",
			entries: []models.Entry{
				{AccountID: "1000", DebitCents: 1080},
				{AccountID: "6200", CreditCents: 1080},
			},
			want: "",
		},
		{
			name: "transfer between non-expense accounts yields no learning",
			entries: []models.Entry{
				{AccountID: "1000", DebitCents: 5000},
				{AccountID: "2000", CreditCents: 5000},
			},
			want: "",
		},
		{
			name:    "no entries",
			entries: nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickExpenseAccount(tt.entries, types); got != tt.want {
				t.Fatalf("pickExpenseAccount = %q, want %q", got, tt.want)
			}
		})
	}
}
