package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/openb00ks/openb00ks/internal/pipeline"
)

func i64(v int64) *int64 { return &v }
func bp(v bool) *bool    { return &v }

type fakeAI struct{ extract, vendor, classify string }

func (f fakeAI) Complete(_ context.Context, _, _, schema string) (string, error) {
	switch schema {
	case pipeline.ExtractSchema:
		return f.extract, nil
	case pipeline.VendorSchema:
		return f.vendor, nil
	case pipeline.ClassifySchema:
		return f.classify, nil
	default:
		return "", errors.New("unexpected schema")
	}
}

func TestScoreCase(t *testing.T) {
	res := pipeline.RunResult{
		Status:   "ready",
		Extract:  pipeline.ExtractResult{TotalCents: i64(1080)},
		Classify: &pipeline.ClassifyResult{AccountCode: "6200"},
	}
	c := Case{WantTotalCents: i64(1080), WantAccountCode: "6200", WantReady: bp(true)}
	s := ScoreCase(c, res)
	if s.Total == nil || !*s.Total || s.Account == nil || !*s.Account || s.Ready == nil || !*s.Ready {
		t.Fatalf("all dimensions should be correct: %+v", s)
	}

	// Wrong (but valid) account → account wrong, still ready.
	res.Classify.AccountCode = "6100"
	s = ScoreCase(c, res)
	if s.Account == nil || *s.Account {
		t.Fatalf("account should score wrong, got %+v", s.Account)
	}
	if s.Ready == nil || !*s.Ready {
		t.Fatalf("ready expectation should still hold")
	}

	// A dimension with no gold isn't scored.
	if ScoreCase(Case{}, res).Total != nil {
		t.Fatalf("no gold total → not scored")
	}
}

func TestRunCaseAndAggregate(t *testing.T) {
	accounts := []pipeline.AccountRef{{Code: "6200", Name: "Software"}, {Code: "6100", Name: "Meals"}}
	base := Case{
		OCRText: "Blue Bottle\nTotal 10.80", Accounts: accounts, CreditAccount: "2000",
		WantTotalCents: i64(1080), WantReady: bp(true), WantVendorName: "Blue Bottle Coffee",
	}
	good := base
	good.Name = "good"
	good.WantAccountCode = "6200"
	goodAI := fakeAI{
		extract:  `{"vendor_name":"Blue Bottle","total_cents":1080,"line_items":[],"confidence":0.95}`,
		vendor:   `{"vendor_id":null,"is_new_vendor":true,"proposed_vendor":{"name":"Blue Bottle Coffee","match_pattern":"BLUE BOTTLE","tax_id":null,"website":null},"confidence":0.9,"reason":"x"}`,
		classify: `{"account_code":"6200","confidence":0.9,"reason":"x"}`,
	}
	_, s1, err := RunCase(context.Background(), goodAI, good)
	if err != nil {
		t.Fatalf("RunCase good: %v", err)
	}

	wrong := base
	wrong.Name = "wrong-account"
	wrong.WantAccountCode = "6200"
	wrongAI := goodAI
	wrongAI.classify = `{"account_code":"6100","confidence":0.9,"reason":"x"}` // valid but wrong account
	_, s2, err := RunCase(context.Background(), wrongAI, wrong)
	if err != nil {
		t.Fatalf("RunCase wrong: %v", err)
	}

	rep := Aggregate([]CaseScore{s1, s2})
	if rep.Cases != 2 || rep.Errors != 0 {
		t.Fatalf("report cases/errors wrong: %+v", rep)
	}
	if rep.Account.Scored != 2 || rep.Account.Correct != 1 || rep.Account.Pct() != 50 {
		t.Fatalf("account accuracy should be 1/2 (50%%), got %+v", rep.Account)
	}
	if rep.Total.Correct != 2 || rep.Total.Pct() != 100 {
		t.Fatalf("total should be 2/2, got %+v", rep.Total)
	}
	if rep.Vendor.Correct != 2 {
		t.Fatalf("both should resolve the proposed vendor name, got %+v", rep.Vendor)
	}
}
