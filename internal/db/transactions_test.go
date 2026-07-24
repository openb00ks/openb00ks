package db

import (
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestValidateBalanced(t *testing.T) {
	if err := validateBalanced([]models.DraftEntry{{DebitCents: 100, CreditCents: 0}, {DebitCents: 0, CreditCents: 100}}); err != nil {
		t.Fatalf("expected balanced, got %v", err)
	}
	if err := validateBalanced([]models.DraftEntry{{DebitCents: 100, CreditCents: 0}}); err == nil {
		t.Fatal("expected error for unbalanced")
	}
	if err := validateBalanced(nil); err == nil {
		t.Fatal("expected error for empty entries")
	}
}

func TestNullString(t *testing.T) {
	if ns := nullString(""); ns.Valid {
		t.Fatal("expected invalid null string")
	}
	if ns := nullString("memo"); !ns.Valid || ns.String != "memo" {
		t.Fatalf("expected memo, got %+v", ns)
	}
}
