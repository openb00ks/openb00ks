package main

import (
	"context"
	"strings"
	"testing"

	"github.com/openb00ks/openb00ks/internal/models"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	ai "github.com/spectrum-labs-tech/ai"
)

func TestBuildEntriesFromSuggestionWithDefaultCash(t *testing.T) {
	receipt := models.Receipt{TotalCents: 1200}
	suggestion := models.ReceiptSuggestion{
		ParsedJSON: []byte(`{"account_id":"expense-1","total_cents":1200}`),
	}
	entries := buildEntriesFromSuggestion(receipt, suggestion, "cash-1")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].AccountID != "expense-1" || entries[0].DebitCents != 1200 {
		t.Fatalf("unexpected debit entry: %+v", entries[0])
	}
	if entries[1].AccountID != "cash-1" || entries[1].CreditCents != 1200 {
		t.Fatalf("unexpected credit entry: %+v", entries[1])
	}
}

func TestBuildEntriesFromSuggestionUsesEntriesArray(t *testing.T) {
	receipt := models.Receipt{}
	suggestion := models.ReceiptSuggestion{
		ParsedJSON: []byte(`{"entries":[{"account_id":"a1","debit_cents":500},{"account_id":"a2","credit_cents":500}]}`),
	}
	entries := buildEntriesFromSuggestion(receipt, suggestion, "cash-1")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].AccountID != "a1" || entries[1].AccountID != "a2" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestRuleMatchCandidates(t *testing.T) {
	got := ruleMatchCandidates("  Acme  ", "", "Acme", "Store", "Store")
	if len(got) != 2 {
		t.Fatalf("expected 2 unique candidates, got %d (%v)", len(got), got)
	}
	if got[0] != "Acme" || got[1] != "Store" {
		t.Fatalf("unexpected candidates: %v", got)
	}
}

func TestAccountRoleHintTokens(t *testing.T) {
	accounts := []models.Account{
		{RoleTags: []string{"utilities", "internet"}},
		{RoleTags: []string{"cell_phone", "internet"}},
	}

	got := accountRoleHintTokens(accounts)
	if len(got) != 3 {
		t.Fatalf("expected 3 hints, got %v", got)
	}
	if got[0] != "cell_phone" || got[1] != "internet" || got[2] != "utilities" {
		t.Fatalf("unexpected hints: %v", got)
	}
}

type fakeAIProvider struct {
	lastSystem string
	lastUser   string
	response   string
}

func (f *fakeAIProvider) Complete(_ context.Context, systemPrompt, userPrompt, _ string, _ ai.Options) (string, error) {
	f.lastSystem = systemPrompt
	f.lastUser = userPrompt
	return f.response, nil
}

func (f *fakeAIProvider) ProviderName() string { return "openai" }
func (f *fakeAIProvider) ModelName() string    { return "gpt-4o-mini" }
func (f *fakeAIProvider) Close() error         { return nil }

func TestCompleteReceiptSuggestionUsesAccountContextAndValidatesAccounts(t *testing.T) {
	t.Parallel()

	provider := &fakeAIProvider{
		response: `{"account_id":"acct-1","explanation":"Internet expense","confidence":0.8}`,
	}
	confidence, parsed, err := completeReceiptSuggestion(
		context.Background(),
		provider,
		models.Receipt{ID: "receipt-1", OriginalName: "Phone bill", TotalCents: 1234, Kind: "receipt"},
		"Entity context line",
		"Vendor note",
		"Extracted text",
		[]string{"internet", "utilities"},
		[]models.Account{
			{ID: "acct-1", Name: "Internet", Type: "expense", RoleTags: []string{"internet"}},
			{ID: "acct-2", Name: "Utilities", Type: "expense", RoleTags: []string{"utilities"}},
		},
		[]searchpkg.Candidate{
			{TransactionID: "tx-prior", AccountID: "acct-1", AccountName: "Internet", Memo: "Comcast", AmountCents: 1234, Score: 0.91},
		},
	)
	if err != nil {
		t.Fatalf("completeReceiptSuggestion() error = %v", err)
	}
	if confidence != 0.8 {
		t.Fatalf("confidence = %v, want 0.8", confidence)
	}
	if parsed["account_id"] != "acct-1" {
		t.Fatalf("parsed account_id = %#v", parsed["account_id"])
	}
	if !strings.Contains(provider.lastSystem, "Chart of accounts:") || !strings.Contains(provider.lastSystem, "tags=internet") {
		t.Fatalf("system prompt missing account context: %s", provider.lastSystem)
	}
	if !strings.Contains(provider.lastSystem, "Relevant account role tags: internet, utilities") {
		t.Fatalf("system prompt missing role tags: %s", provider.lastSystem)
	}
	if !strings.Contains(provider.lastUser, "Receipt ID: receipt-1") {
		t.Fatalf("user prompt missing receipt id: %s", provider.lastUser)
	}
	if !strings.Contains(provider.lastSystem, "Accepted historical matches:") || !strings.Contains(provider.lastSystem, "tx-prior | account=acct-1") {
		t.Fatalf("system prompt missing historical candidates: %s", provider.lastSystem)
	}
}

func TestCompleteReceiptSuggestionRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	provider := &fakeAIProvider{
		response: `{"account_id":"acct-unknown","explanation":"Wrong account","confidence":0.8}`,
	}
	confidence, parsed, err := completeReceiptSuggestion(
		context.Background(),
		provider,
		models.Receipt{ID: "receipt-1", OriginalName: "Phone bill", TotalCents: 1234, Kind: "receipt"},
		"",
		"",
		"",
		nil,
		[]models.Account{{ID: "acct-1", Name: "Internet", Type: "expense"}},
		nil,
	)
	if err != nil {
		t.Fatalf("completeReceiptSuggestion() error = %v", err)
	}
	if confidence != 0 || parsed != nil {
		t.Fatalf("expected unknown account to be rejected, got confidence=%v parsed=%#v", confidence, parsed)
	}
}

func TestHistoricalCandidatePromptContext(t *testing.T) {
	t.Parallel()

	got := historicalCandidatePromptContext([]searchpkg.Candidate{
		{TransactionID: "tx-1", AccountID: "acct-1", AccountName: "Utilities", Memo: "Power bill", AmountCents: 4500, Score: 0.9},
	})
	if !strings.Contains(got, "tx-1 | account=acct-1 Utilities") {
		t.Fatalf("missing account context: %q", got)
	}
	if !strings.Contains(got, "amount_cents=4500") || !strings.Contains(got, "score=0.90") {
		t.Fatalf("missing amount/score context: %q", got)
	}
}

func TestSuggestionSearchQueryIsCompact(t *testing.T) {
	t.Parallel()

	query := suggestionSearchQuery("receipt.jpg", "home internet", strings.Repeat("x", 800), "Comcast", []string{"internet"})
	if !strings.Contains(query, "receipt.jpg") || !strings.Contains(query, "Comcast") || !strings.Contains(query, "internet") {
		t.Fatalf("query missing expected signals: %q", query)
	}
	if len(query) > 620 {
		t.Fatalf("query should be compact, got length %d", len(query))
	}
}

func TestDominantAccount(t *testing.T) {
	counts := map[string]int{
		"acct-expense": 3,
		"acct-other":   1,
	}
	if got := dominantAccount(counts); got != "acct-expense" {
		t.Fatalf("expected acct-expense, got %q", got)
	}

	tied := map[string]int{
		"b": 2,
		"a": 2,
	}
	if got := dominantAccount(tied); got != "a" {
		t.Fatalf("expected lexicographically smaller account in tie, got %q", got)
	}
}

func TestBuildEntriesFromSuggestionImportRows(t *testing.T) {
	receipt := models.Receipt{Kind: "import"}
	suggestion := models.ReceiptSuggestion{
		ParsedJSON: []byte(`{
			"account_id":"fallback-expense",
			"import_rows":[
				{"row_index":1,"account_id":"meals","amount_cents":1200},
				{"row_index":2,"amount_cents":800},
				{"row_index":3,"account_id":"travel","amount_cents":500}
			]
		}`),
	}

	entries := buildEntriesFromSuggestion(receipt, suggestion, "cash")
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].AccountID != "fallback-expense" || entries[0].DebitCents != 800 {
		t.Fatalf("unexpected fallback debit entry: %+v", entries[0])
	}
	if entries[1].AccountID != "meals" || entries[1].DebitCents != 1200 {
		t.Fatalf("unexpected meals debit entry: %+v", entries[1])
	}
	if entries[2].AccountID != "travel" || entries[2].DebitCents != 500 {
		t.Fatalf("unexpected travel debit entry: %+v", entries[2])
	}
	if entries[3].AccountID != "cash" || entries[3].CreditCents != 2500 {
		t.Fatalf("unexpected credit entry: %+v", entries[3])
	}
}

func TestBuildEntriesFromSuggestionImportRowsRequiresCreditAccount(t *testing.T) {
	receipt := models.Receipt{Kind: "import"}
	suggestion := models.ReceiptSuggestion{
		ParsedJSON: []byte(`{
			"import_rows":[
				{"row_index":1,"account_id":"meals","amount_cents":1200}
			]
		}`),
	}

	entries := buildEntriesFromSuggestion(receipt, suggestion, "")
	if len(entries) != 0 {
		t.Fatalf("expected no entries without credit account, got %+v", entries)
	}
}
