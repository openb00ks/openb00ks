package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/models"
)

type fakeAccountStatementStore struct {
	rows            []models.AccountStatement
	created         models.AccountStatement
	updatedID       string
	updatedPatch    db.AccountStatementPatch
	reconciledID    string
	bySourceReceipt map[string]models.AccountStatement
	entityByID      map[string]string
}

func (f *fakeAccountStatementStore) List(ctx context.Context, entityID, accountID string, start, end *time.Time, limit int) ([]models.AccountStatement, error) {
	out := make([]models.AccountStatement, 0, len(f.rows))
	for _, row := range f.rows {
		if entityID != "" && row.EntityID != entityID {
			continue
		}
		if accountID != "" && row.AccountID != accountID {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (f *fakeAccountStatementStore) GetByID(ctx context.Context, id string) (models.AccountStatement, error) {
	for _, row := range f.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return models.AccountStatement{}, db.ErrNotFound
}

func (f *fakeAccountStatementStore) GetBySourceReceiptID(ctx context.Context, receiptID string) (models.AccountStatement, error) {
	if f.bySourceReceipt != nil {
		if row, ok := f.bySourceReceipt[receiptID]; ok {
			return row, nil
		}
	}
	return models.AccountStatement{}, db.ErrNotFound
}

func (f *fakeAccountStatementStore) GetEntityID(ctx context.Context, id string) (string, error) {
	if f.entityByID != nil {
		if entityID, ok := f.entityByID[id]; ok {
			return entityID, nil
		}
	}
	for _, row := range f.rows {
		if row.ID == id {
			return row.EntityID, nil
		}
	}
	return "", db.ErrNotFound
}

func (f *fakeAccountStatementStore) Create(ctx context.Context, statement models.AccountStatement) (models.AccountStatement, error) {
	f.created = statement
	statement.ID = "stmt-created"
	statement.AccountName = "Operating"
	statement.ExpectedEndingBalanceCents = db.StatementExpectedEndingBalance(statement.StartingBalanceCents, 25_00, 10_00)
	statement.DifferenceCents = db.StatementDifferenceCents(statement.EndingBalanceCents, statement.ExpectedEndingBalanceCents)
	return statement, nil
}

func (f *fakeAccountStatementStore) Update(ctx context.Context, id string, patch db.AccountStatementPatch) (models.AccountStatement, error) {
	f.updatedID = id
	f.updatedPatch = patch
	row, err := f.GetByID(ctx, id)
	if err != nil {
		return models.AccountStatement{}, err
	}
	if patch.Status != nil {
		row.Status = *patch.Status
	}
	return row, nil
}

func (f *fakeAccountStatementStore) Reconcile(ctx context.Context, id string) (models.AccountStatement, error) {
	f.reconciledID = id
	row, err := f.GetByID(ctx, id)
	if err != nil {
		return models.AccountStatement{}, err
	}
	if row.DifferenceCents == 0 && row.UnpostedRows == 0 {
		row.Status = "reconciled"
	} else {
		row.Status = "needs_review"
	}
	return row, nil
}

func newTestGinContext(method, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, w
}

func TestHandleAccountStatementCreate(t *testing.T) {
	t.Parallel()

	store := &fakeAccountStatementStore{}
	hc := &HandlerContext{
		accountStatements: store,
		accounts: fakeAccountStore{
			entityByAccount: map[string]string{"acct-1": "entity-1"},
		},
		receiptStore: &fakeReceiptStore{
			receipts: map[string]models.Receipt{
				"import-1": {ID: "import-1", EntityID: "entity-1", Kind: "import", OriginalName: "bank.csv"},
			},
		},
	}
	c, w := newTestGinContext(http.MethodPost, "/account-statements", map[string]any{
		"entity_id":              "entity-1",
		"account_id":             "acct-1",
		"source_receipt_id":      "import-1",
		"period_start":           "2026-01-01",
		"period_end":             "2026-01-31",
		"starting_balance_cents": 10_000,
		"ending_balance_cents":   11_500,
		"notes":                  "January statement",
	})

	hc.handleAccountStatementCreate(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if store.created.EntityID != "entity-1" || store.created.AccountID != "acct-1" || store.created.SourceReceiptID != "import-1" {
		t.Fatalf("unexpected created statement: %#v", store.created)
	}
	if store.created.PeriodStart.Format("2006-01-02") != "2026-01-01" || store.created.PeriodEnd.Format("2006-01-02") != "2026-01-31" {
		t.Fatalf("unexpected statement period: %#v", store.created)
	}
}

func TestHandleAccountStatementCreateRejectsWrongEntityAccount(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accountStatements: &fakeAccountStatementStore{},
		accounts: fakeAccountStore{
			entityByAccount: map[string]string{"acct-1": "other-entity"},
		},
	}
	c, w := newTestGinContext(http.MethodPost, "/account-statements", map[string]any{
		"entity_id":              "entity-1",
		"account_id":             "acct-1",
		"period_start":           "2026-01-01",
		"period_end":             "2026-01-31",
		"starting_balance_cents": 10_000,
		"ending_balance_cents":   11_500,
	})

	hc.handleAccountStatementCreate(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAccountStatementReconcile(t *testing.T) {
	t.Parallel()

	store := &fakeAccountStatementStore{
		rows: []models.AccountStatement{{
			ID:              "stmt-1",
			EntityID:        "entity-1",
			AccountID:       "acct-1",
			AccountName:     "Operating",
			DifferenceCents: 500,
			UnpostedRows:    1,
			Status:          "draft",
			PeriodStart:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:       time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		}},
	}
	hc := &HandlerContext{accountStatements: store}
	c, w := newTestGinContext(http.MethodPost, "/account-statements/stmt-1/reconcile", nil)
	c.Params = gin.Params{{Key: "id", Value: "stmt-1"}}

	hc.handleAccountStatementReconcile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.reconciledID != "stmt-1" {
		t.Fatalf("expected reconcile id stmt-1, got %q", store.reconciledID)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"status":"needs_review"`)) {
		t.Fatalf("expected needs_review response, got %s", w.Body.String())
	}
}

func TestFiscalYearRangeUsesEntityFiscalStart(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		entities: &fakeEntityStore{
			entities: map[string]models.Entity{
				"entity-1": {
					ID:                   "entity-1",
					FiscalYearStartMonth: 4,
					FiscalYearStartDay:   1,
				},
			},
		},
	}

	start, end := hc.fiscalYearRange(context.Background(), "entity-1", 2025)
	if start.Format("2006-01-02") != "2025-04-01" || end.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("unexpected fiscal range: %s to %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func TestTaxStatementExceptionsAndRows(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	hc := &HandlerContext{
		accountStatements: &fakeAccountStatementStore{
			rows: []models.AccountStatement{{
				ID:                         "stmt-1",
				EntityID:                   "entity-1",
				AccountID:                  "acct-1",
				AccountName:                "Operating",
				SourceReceiptID:            "import-1",
				SourceReceiptName:          "bank.csv",
				PeriodStart:                start,
				PeriodEnd:                  time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
				StartingBalanceCents:       10_000,
				EndingBalanceCents:         11_000,
				ImportedInflowCents:        2_500,
				ImportedOutflowCents:       1_000,
				PostedInflowCents:          2_500,
				ExpectedEndingBalanceCents: 11_500,
				DifferenceCents:            -500,
				UnpostedRows:               1,
				Status:                     "draft",
			}},
		},
	}
	c, _ := newTestGinContext(http.MethodGet, "/reports/tax-readiness", nil)

	exceptions := hc.taxStatementExceptions(c, "entity-1", start, end)
	if len(exceptions) != 3 {
		t.Fatalf("expected three statement blockers, got %#v", exceptions)
	}
	if exceptions[0].Kind != "account_statement" || exceptions[0].Issue != "statement balance difference" || exceptions[0].Amount != "-500" {
		t.Fatalf("unexpected first statement exception: %#v", exceptions[0])
	}
	if href := taxExceptionHref(exceptions[0]); href != "/statements" {
		t.Fatalf("unexpected statement href: %q", href)
	}

	rows := hc.taxStatementRows(c, "entity-1", start, end)
	if len(rows) != 2 {
		t.Fatalf("statement rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "stmt-1" || rows[1][14] != "-500" || rows[1][15] != "1" {
		t.Fatalf("unexpected statement CSV row: %#v", rows[1])
	}
}

func TestImportReceiptOffsetAccountUsesStatementAccount(t *testing.T) {
	t.Parallel()

	hc := &HandlerContext{
		accountStatements: &fakeAccountStatementStore{
			bySourceReceipt: map[string]models.AccountStatement{
				"import-1": {EntityID: "entity-1", AccountID: "statement-account"},
			},
		},
		accounts: fakeAccountStore{
			defaultCash: models.Account{ID: "cash-account", EntityID: "entity-1"},
		},
	}
	c, _ := newTestGinContext(http.MethodPost, "/imports/import-1/rows/1/post", nil)

	got, err := hc.importReceiptOffsetAccountID(c, "import-1", "entity-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "statement-account" {
		t.Fatalf("expected statement offset account, got %q", got)
	}
}
