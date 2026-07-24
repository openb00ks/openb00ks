package ocr

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spectrum-labs-tech/ai"
)

// fakeProvider is a stand-in ai.Provider that records the call and returns a canned response.
type fakeProvider struct {
	gotSystem, gotUser, gotSchema string
	gotOpts                       ai.Options
	resp                          string
	err                           error
}

func (f *fakeProvider) Complete(_ context.Context, system, user, schema string, opts ai.Options) (string, error) {
	f.gotSystem, f.gotUser, f.gotSchema, f.gotOpts = system, user, schema, opts
	return f.resp, f.err
}
func (f *fakeProvider) ProviderName() string { return "fake" }
func (f *fakeProvider) ModelName() string    { return "fake-vision" }
func (f *fakeProvider) Close() error         { return nil }

func TestLLMVision_SendsImageAndReturnsText(t *testing.T) {
	fp := &fakeProvider{resp: "  ACME STORE\nTotal $12.34  "}
	tr := NewLLMVision(fp, 2048)

	res, err := tr.Transcribe(context.Background(), "https://r2.example/receipt.png?sig=abc", "image/png")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Text != "ACME STORE\nTotal $12.34" {
		t.Fatalf("text = %q (want trimmed transcription)", res.Text)
	}
	if res.Provider != "llm-vision" || res.Model != "fake-vision" {
		t.Fatalf("provider/model = %q/%q", res.Provider, res.Model)
	}
	// The image must be attached and the call must be deterministic (temperature 0), schema-free.
	if fp.gotOpts.ImageURL != "https://r2.example/receipt.png?sig=abc" {
		t.Fatalf("image not attached: %q", fp.gotOpts.ImageURL)
	}
	if fp.gotOpts.Temperature == nil || *fp.gotOpts.Temperature != 0 {
		t.Fatalf("expected temperature 0, got %v", fp.gotOpts.Temperature)
	}
	if fp.gotSchema != "" {
		t.Fatalf("transcription should be schema-free, got %q", fp.gotSchema)
	}
	if !strings.Contains(fp.gotSystem, "OCR") {
		t.Fatalf("system prompt missing OCR framing")
	}
}

func TestLLMVision_RejectsNonFetchableURL(t *testing.T) {
	tr := NewLLMVision(&fakeProvider{resp: "x"}, 0)
	if _, err := tr.Transcribe(context.Background(), "20260719/receipt.png", "image/png"); err == nil {
		t.Fatal("expected error for a bare local-storage key (not fetchable by the model)")
	}
}

func TestLLMVision_PropagatesProviderError(t *testing.T) {
	tr := NewLLMVision(&fakeProvider{err: errors.New("rate limited")}, 0)
	if _, err := tr.Transcribe(context.Background(), "https://x/y.png", "image/png"); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

func TestNoop_ReturnsEmpty(t *testing.T) {
	res, err := Noop{}.Transcribe(context.Background(), "https://x/y.png", "image/png")
	if err != nil || res.Text != "" || res.Provider != "none" {
		t.Fatalf("noop = %+v, err=%v", res, err)
	}
}
