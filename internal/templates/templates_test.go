package templates

import "testing"

// typeCodePrefix maps an account type to the leading digit its chart-of-accounts code must use
// (assets 1xxx, liabilities 2xxx, equity 3xxx, income 4xxx, expenses 5xxx).
var typeCodePrefix = map[string]byte{
	"asset":     '1',
	"liability": '2',
	"equity":    '3',
	"income":    '4',
	"expense":   '5',
}

// TestTemplateAccountCodes asserts every seeded account carries a well-formed, unique code whose
// leading digit matches its type class, so the seeded chart of accounts orders and reads correctly.
func TestTemplateAccountCodes(t *testing.T) {
	for _, tmpl := range List() {
		seen := map[string]string{}
		for _, acct := range tmpl.Accounts {
			if acct.Code == "" {
				t.Errorf("%s: account %q missing code", tmpl.Key, acct.Name)
				continue
			}
			if len(acct.Code) != 4 {
				t.Errorf("%s: account %q code %q is not 4 digits", tmpl.Key, acct.Name, acct.Code)
			}
			for _, r := range acct.Code {
				if r < '0' || r > '9' {
					t.Errorf("%s: account %q code %q is not numeric", tmpl.Key, acct.Name, acct.Code)
					break
				}
			}
			if want := typeCodePrefix[acct.Type]; want != 0 && acct.Code[0] != want {
				t.Errorf("%s: %s account %q code %q should start with %c", tmpl.Key, acct.Type, acct.Name, acct.Code, want)
			}
			if prev, dup := seen[acct.Code]; dup {
				t.Errorf("%s: code %q reused by %q and %q", tmpl.Key, acct.Code, prev, acct.Name)
			}
			seen[acct.Code] = acct.Name
		}
	}
}

func TestLookupBasicTemplate(t *testing.T) {
	tmpl, err := Lookup("basic")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if tmpl.Key != "basic" {
		t.Fatalf("expected key basic, got %q", tmpl.Key)
	}
	if tmpl.Name == "" {
		t.Fatal("expected name")
	}
	if len(tmpl.Accounts) == 0 {
		t.Fatal("expected accounts")
	}
	for i, acct := range tmpl.Accounts {
		if acct.Name == "" {
			t.Fatalf("account %d missing name", i)
		}
		if acct.Type == "" {
			t.Fatalf("account %d missing type", i)
		}
	}
}

func TestLookupUnknownTemplate(t *testing.T) {
	if _, err := Lookup("missing-template"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestList(t *testing.T) {
	list := List()
	if len(list) < 5 {
		t.Fatalf("expected at least 5 templates (basic + 4 business types), got %d", len(list))
	}
	// The default (basic) must sort first so the picker defaults to it.
	if list[0].Key != DefaultKey {
		t.Fatalf("expected %q first, got %q", DefaultKey, list[0].Key)
	}
	// Every template is usable: has a name, at least one account, and a Cash account (or one is added
	// at seed time) — plus at least one expense account so the classifier has something to choose.
	byKey := map[string]Template{}
	for _, tmpl := range list {
		if tmpl.Name == "" || len(tmpl.Accounts) == 0 {
			t.Fatalf("template %q missing name/accounts", tmpl.Key)
		}
		expenses := 0
		for _, a := range tmpl.Accounts {
			if a.Type == "expense" {
				expenses++
			}
		}
		if expenses == 0 {
			t.Fatalf("template %q has no expense accounts", tmpl.Key)
		}
		byKey[tmpl.Key] = tmpl
	}
	for _, want := range []string{"basic", "software-startup", "property-management", "short-term-rental", "small-retailer"} {
		if _, ok := byKey[want]; !ok {
			t.Errorf("expected template %q to be listed", want)
		}
	}
	// Rest are alphabetical by name after basic.
	for i := 2; i < len(list); i++ {
		if list[i-1].Name > list[i].Name {
			t.Errorf("templates after basic not name-sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
}
