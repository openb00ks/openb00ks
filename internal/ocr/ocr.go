// Package ocr is stage 1 of the receipt pipeline: turn a receipt image into plain text. It does
// transcription ONLY — field extraction (vendor, total, line items) is a later, separately-validated
// stage (see docs/receipt-pipeline.md). Keeping transcription and extraction apart shrinks each AI
// ask and makes both debuggable.
package ocr

import (
	"context"
	"fmt"
	"strings"

	"github.com/spectrum-labs-tech/ai"
)

// Result is a receipt transcription and the provider/model that produced it.
type Result struct {
	Text     string
	Provider string
	Model    string
}

// Transcriber turns a receipt image into text. Implementations must be safe for concurrent use.
type Transcriber interface {
	// Transcribe returns the plain-text transcription of the image at imageURL. contentType is the
	// image MIME type (advisory).
	Transcribe(ctx context.Context, imageURL, contentType string) (Result, error)
}

// Noop returns empty text — used when OCR is disabled (OCR_PROVIDER=none). The pipeline then parks the
// receipt for manual entry rather than fabricating content.
type Noop struct{}

func (Noop) Transcribe(context.Context, string, string) (Result, error) {
	return Result{Provider: "none"}, nil
}

const transcribeSystem = `You are a precise OCR engine for financial receipts and invoices.
Transcribe ALL text visible in the image EXACTLY as printed — every line, every amount, every date.
Preserve the reading order and line breaks. Do NOT summarize, interpret, translate, reformat, or add
commentary. Do NOT compute or infer any value that is not printed. Output ONLY the transcribed text.`

const transcribeUser = `Transcribe this receipt image.`

// LLMVision transcribes by sending the image to a vision-capable model through the shared AI driver.
// The provider must be configured with a vision model, and imageURL must be fetchable by that provider
// (a presigned object-storage URL — so llm-vision requires RECEIPT_STORAGE=s3).
type LLMVision struct {
	provider  ai.Provider
	maxTokens int
}

// NewLLMVision wraps a vision-capable provider. The caller owns the provider's lifecycle (Close).
func NewLLMVision(provider ai.Provider, maxTokens int) *LLMVision {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &LLMVision{provider: provider, maxTokens: maxTokens}
}

func (t *LLMVision) Transcribe(ctx context.Context, imageURL, _ string) (Result, error) {
	if imageURL == "" {
		return Result{}, fmt.Errorf("ocr: empty image URL")
	}
	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		// A bare local-storage key is not fetchable by the model. llm-vision needs object storage.
		return Result{}, fmt.Errorf("ocr: llm-vision needs a fetchable image URL (set RECEIPT_STORAGE=s3); got %q", imageURL)
	}
	zero := 0.0
	text, err := t.provider.Complete(ctx, transcribeSystem, transcribeUser, "", ai.Options{
		Temperature: &zero, // determinism — transcription must not vary
		MaxTokens:   t.maxTokens,
		ImageURL:    imageURL,
	})
	if err != nil {
		return Result{}, fmt.Errorf("ocr: vision transcription failed: %w", err)
	}
	return Result{
		Text:     strings.TrimSpace(text),
		Provider: "llm-vision",
		Model:    t.provider.ModelName(),
	}, nil
}
