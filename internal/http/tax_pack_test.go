package httpapi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type fakeEntityTaxSettingsStore struct {
	settings db.EntityTaxSettings
}

func (f fakeEntityTaxSettingsStore) Get(_ context.Context, _ string, _ int) (db.EntityTaxSettings, error) {
	return f.settings, nil
}

func (f fakeEntityTaxSettingsStore) Upsert(_ context.Context, _ string, _ int, _, _, _, _ sql.NullInt64) (db.EntityTaxSettings, error) {
	return f.settings, nil
}

type fakeAccountStore struct {
	accounts        []models.Account
	entityByAccount map[string]string
	defaultCash     models.Account
}

func (f fakeAccountStore) ListForEntity(_ context.Context, _ string, _ int) ([]models.Account, error) {
	return f.accounts, nil
}

func (f fakeAccountStore) GetByID(_ context.Context, accountID string) (models.Account, error) {
	for _, account := range f.accounts {
		if account.ID == accountID {
			return account, nil
		}
	}
	return models.Account{}, db.ErrNotFound
}

func (fakeAccountStore) Create(context.Context, string, string, string, string, ...string) (models.Account, error) {
	return models.Account{}, nil
}

func (fakeAccountStore) Update(context.Context, string, string, string, string, ...string) (models.Account, error) {
	return models.Account{}, nil
}

func (fakeAccountStore) Delete(context.Context, string) error {
	return nil
}

func (f fakeAccountStore) GetEntityID(_ context.Context, accountID string) (string, error) {
	if f.entityByAccount != nil {
		if entityID, ok := f.entityByAccount[accountID]; ok {
			return entityID, nil
		}
	}
	return "", nil
}

func (f fakeAccountStore) FindDefaultCashAccount(context.Context, string) (models.Account, error) {
	return f.defaultCash, nil
}

func TestImportRowsInScopeFiltersToTaxPeriod(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	rows := []models.ImportRow{
		{RowIndex: 1, Date: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
		{RowIndex: 2, Date: start},
		{RowIndex: 3, Date: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)},
		{RowIndex: 4, Date: end},
		{RowIndex: 5, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	got := importRowsInScope(rows, start, end)
	if len(got) != 3 {
		t.Fatalf("filtered rows = %d, want 3: %#v", len(got), got)
	}
	for i, want := range []int{2, 3, 4} {
		if got[i].RowIndex != want {
			t.Fatalf("filtered row[%d] = %d, want %d", i, got[i].RowIndex, want)
		}
	}
}

func TestMapDateInTaxScope(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		row  map[string]interface{}
		want bool
	}{
		"in range": {
			row:  map[string]interface{}{"date": "2025-04-15"},
			want: true,
		},
		"out of range": {
			row:  map[string]interface{}{"date": "2026-01-01"},
			want: false,
		},
		"missing date is retained for manual review": {
			row:  map[string]interface{}{},
			want: true,
		},
		"invalid date is retained for manual review": {
			row:  map[string]interface{}{"date": "not-a-date"},
			want: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := mapDateInTaxScope(tt.row, start, end); got != tt.want {
				t.Fatalf("mapDateInTaxScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaxActionItemsGroupsAndPrioritizesExceptions(t *testing.T) {
	t.Parallel()

	got := taxActionItems([]taxException{
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Issue:      "import row not posted",
		},
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Issue:      "unmapped import row",
		},
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Issue:      "unmapped import row",
		},
	})

	if len(got) != 2 {
		t.Fatalf("actions = %d, want 2: %#v", len(got), got)
	}
	if got[0].Kind != "map_import_rows" || got[0].Count != 2 {
		t.Fatalf("first action = %#v, want grouped map action count 2", got[0])
	}
	if got[0].Href != "/imports/import-1" {
		t.Fatalf("first action href = %q, want import link", got[0].Href)
	}
	if got[1].Kind != "post_import_rows" || got[1].Count != 1 {
		t.Fatalf("second action = %#v, want post action count 1", got[1])
	}
}

func TestTaxActionRowsSerializesActions(t *testing.T) {
	t.Parallel()

	rows := taxActionRows([]taxActionItem{
		{
			Kind:     "map_import_rows",
			Label:    "Map uncategorized rows in bank.csv",
			Count:    2,
			Href:     "/imports/import-1",
			Priority: 10,
		},
	})

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0][0] != "kind" || rows[1][0] != "map_import_rows" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if rows[1][2] != "2" || rows[1][3] != "/imports/import-1" {
		t.Fatalf("unexpected action row: %#v", rows[1])
	}
}

func TestBlockingSummaryRowsGroupsIssuesBySource(t *testing.T) {
	t.Parallel()

	rows := blockingSummaryRows([]taxException{
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Status:     "ready_for_review",
			Issue:      "unmapped import row",
			RowIndex:   "2",
		},
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Status:     "ready_for_review",
			Issue:      "duplicate import row fingerprint",
			RowIndex:   "3",
		},
		{
			SourceID:   "receipt-1",
			SourceName: "receipt.pdf",
			Kind:       "receipt",
			Status:     "uploaded",
			Issue:      "not posted",
		},
	})

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0]["source_id"] != "import-1" {
		t.Fatalf("first row = %#v", rows[0])
	}
	if rows[0]["issue_count"] != 2 || rows[0]["unmapped_rows"] != 1 || rows[0]["duplicate_rows"] != 1 {
		t.Fatalf("unexpected summary row: %#v", rows[0])
	}
	if rows[0]["first_row_index"] != "2" {
		t.Fatalf("unexpected row index: %#v", rows[0])
	}
	if rows[1]["href"] != "/receipts/receipt-1" {
		t.Fatalf("unexpected receipt href: %#v", rows[1])
	}
}

func TestTaxExceptionsRowsIncludesDeepLinks(t *testing.T) {
	t.Parallel()

	rows := taxExceptionsRows([]taxException{
		{
			SourceID:   "import-1",
			SourceName: "bank.csv",
			Kind:       "import_row",
			Status:     "ready_for_review",
			Issue:      "unmapped import row",
			RowIndex:   "2",
		},
		{
			SourceID:   "receipt-1",
			SourceName: "receipt.pdf",
			Kind:       "receipt",
			Status:     "uploaded",
			Issue:      "not posted",
		},
	})

	if rows[1][8] != "/imports/import-1#row-2" {
		t.Fatalf("unexpected import row href: %#v", rows[1])
	}
	if rows[2][8] != "/receipts/receipt-1" {
		t.Fatalf("unexpected receipt href: %#v", rows[2])
	}
}

func TestTaxPrepChecklistRowsIncludesCoreBlocks(t *testing.T) {
	t.Parallel()

	rows := (&HandlerContext{}).taxPrepChecklistRows(nil, "entity-1", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), []taxException{
		{
			SourceID: "import-1",
			Kind:     "import_row",
			Issue:    "unmapped import row",
		},
		{
			SourceID: "receipt-1",
			Kind:     "receipt",
			Issue:    "not posted",
		},
	})

	if rows[0][0] != "section" {
		t.Fatalf("unexpected header row: %#v", rows[0])
	}
	if got := findRowByItem(rows, "Unmapped rows"); got == nil || got[2] != "needs attention" || got[3] != "1" {
		t.Fatalf("unexpected import checklist row: %#v", got)
	}
	if got := findRowByItem(rows, "Mileage trips"); got == nil || got[2] != "ready" {
		t.Fatalf("unexpected mileage checklist row: %#v", got)
	}
}

func TestTaxPreparedPackageRowsSummarizesFilingState(t *testing.T) {
	t.Parallel()

	rows := (&HandlerContext{}).taxPreparedPackageRows(nil, "entity-1", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), []taxException{
		{
			SourceID:   "import-1",
			Kind:       "import_row",
			Issue:      "unmapped import row",
			RowIndex:   "2",
			SourceName: "bank.csv",
		},
	}, [][]string{{"import_id"}, {"import-1"}})

	if rows[0][0] != "item" {
		t.Fatalf("unexpected header row: %#v", rows[0])
	}
	if got := findPackageRowByItem(rows, "Tax pack"); got == nil || got[1] != "needs attention" || got[2] != "1" {
		t.Fatalf("unexpected package status row: %#v", got)
	}
	if got := findPackageRowByItem(rows, "Import sources"); got == nil || got[2] != "1" || got[1] != "ready" {
		t.Fatalf("unexpected import source row: %#v", got)
	}
}

func TestTaxAllocationRowsIncludeHomeUseSettings(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		entityTaxSettings: fakeEntityTaxSettingsStore{
			settings: db.EntityTaxSettings{
				EntityID:                       "entity-1",
				TaxYear:                        2025,
				CreatedAt:                      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:                      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				HomeOfficeSqFt:                 sql.NullInt64{Int64: 250, Valid: true},
				HomeTotalSqFt:                  sql.NullInt64{Int64: 1000, Valid: true},
				CellPhoneBusinessUsePercent:    sql.NullInt64{Int64: 75, Valid: true},
				HomeInternetBusinessUsePercent: sql.NullInt64{Int64: 60, Valid: true},
			},
		},
	}

	rows := hc.taxUseAllocationRows(nil, "entity-1", 2025)
	if len(rows) != 4 {
		t.Fatalf("allocation rows = %d, want 4", len(rows))
	}
	if rows[1][0] != "2025" || rows[1][1] != "Home utilities allocation" || rows[1][3] != "25%" {
		t.Fatalf("unexpected utilities allocation row: %#v", rows[1])
	}
	if rows[2][3] != "75%" || rows[3][3] != "60%" {
		t.Fatalf("unexpected percentage rows: %#v %#v", rows[2], rows[3])
	}
}

func TestTaxUseProfileExceptionsAddEntityBlocker(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		entityTaxSettings: fakeEntityTaxSettingsStore{
			settings: db.EntityTaxSettings{
				EntityID:                    "entity-1",
				TaxYear:                     2025,
				CreatedAt:                   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:                   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				CellPhoneBusinessUsePercent: sql.NullInt64{Int64: 75, Valid: true},
			},
		},
	}

	exceptions := hc.taxUseProfileExceptions(nil, "entity-1", 2025)
	if len(exceptions) != 1 {
		t.Fatalf("expected one blocker, got %#v", exceptions)
	}
	if exceptions[0].Kind != "entity" || exceptions[0].Issue != "partial home-use allocation" {
		t.Fatalf("unexpected blocker: %#v", exceptions[0])
	}
	if href := taxExceptionHref(exceptions[0]); href != "/settings/entity" {
		t.Fatalf("unexpected href: %s", href)
	}
}

func TestTaxAccountRoleCoverageCountsTaggedAccounts(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accounts: fakeAccountStore{
			accounts: []models.Account{
				{ID: "acct-1", Name: "Utilities", Type: "expense", RoleTags: []string{"utilities", "internet"}},
				{ID: "acct-2", Name: "Phone", Type: "expense", RoleTags: []string{"cell_phone"}},
			},
		},
	}

	coverage := hc.loadAccountRoleCoverage(nil, "entity-1")
	if coverage.UtilitiesCount != 1 || coverage.CellPhoneCount != 1 || coverage.InternetCount != 1 {
		t.Fatalf("unexpected coverage: %#v", coverage)
	}
	if coverage.Href != "/accounts" {
		t.Fatalf("unexpected href: %#v", coverage)
	}
}

func TestTaxAccountRoleCoverageResponseSerializesCounts(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accounts: fakeAccountStore{
			accounts: []models.Account{
				{ID: "acct-1", Name: "Utilities", Type: "expense", RoleTags: []string{"utilities"}},
			},
		},
	}

	resp := hc.taxAccountRoleCoverageResponse(nil, "entity-1")
	if resp["utilities_count"] != 1 || resp["cell_phone_count"] != 0 || resp["internet_count"] != 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp["href"] != "/accounts" {
		t.Fatalf("unexpected response href: %#v", resp)
	}
}

func TestTaxAccountRoleExceptionsRequireTaggedAccountsForAllocations(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accounts: fakeAccountStore{
			accounts: []models.Account{},
		},
		entityTaxSettings: fakeEntityTaxSettingsStore{
			settings: db.EntityTaxSettings{
				EntityID:                       "entity-1",
				TaxYear:                        2025,
				CreatedAt:                      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:                      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				HomeOfficeSqFt:                 sql.NullInt64{Int64: 250, Valid: true},
				HomeTotalSqFt:                  sql.NullInt64{Int64: 1000, Valid: true},
				CellPhoneBusinessUsePercent:    sql.NullInt64{Int64: 75, Valid: true},
				HomeInternetBusinessUsePercent: sql.NullInt64{Int64: 60, Valid: true},
			},
		},
	}

	exceptions := hc.taxAccountRoleExceptions(nil, "entity-1", 2025)
	if len(exceptions) != 3 {
		t.Fatalf("expected three blockers, got %#v", exceptions)
	}
	issues := map[string]bool{}
	for _, item := range exceptions {
		issues[item.Issue] = true
	}
	for _, want := range []string{
		"missing utilities tagged account",
		"missing cell phone tagged account",
		"missing internet tagged account",
	} {
		if !issues[want] {
			t.Fatalf("missing blocker %q in %#v", want, exceptions)
		}
	}
}

func TestAccountRoleTagRowsIncludeRoleTags(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accounts: fakeAccountStore{
			accounts: []models.Account{
				{ID: "acct-1", Name: "Utilities", Type: "expense", RoleTags: []string{"utilities", "internet"}},
			},
		},
	}

	rows := hc.accountRoleTagRows(nil, "entity-1")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "acct-1" || rows[1][3] != "utilities,internet" {
		t.Fatalf("unexpected tagged account row: %#v", rows[1])
	}
}

func findRowByItem(rows [][]string, item string) []string {
	for _, row := range rows {
		if len(row) > 1 && row[1] == item {
			return row
		}
	}
	return nil
}

func findPackageRowByItem(rows [][]string, item string) []string {
	for _, row := range rows {
		if len(row) > 0 && row[0] == item {
			return row
		}
	}
	return nil
}
