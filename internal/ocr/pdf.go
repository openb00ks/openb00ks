package ocr

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Tier-1 OCR for PDFs: pull the embedded text layer out locally, with NO AI call. Digital receipts and
// invoices (the common case) carry a real text layer, so this transcribes them for free; a scanned or
// image-only PDF has no text layer and yields little or nothing, which SufficientText detects so the
// caller can escalate to the AI tier. Keeping this separate from LLMVision (which speaks image_url and
// can't take a PDF) is what lets the pipeline try the cheap, deterministic path first.

const (
	// minReceiptTextRunes — below this, the extracted text is too thin to be a real receipt.
	minReceiptTextRunes = 40
	// minReceiptDigits — a receipt has amounts/dates, so it must carry at least a few numerals; this
	// rejects a page of stray ligatures scraped off a mostly-image PDF.
	minReceiptDigits = 3
)

// ExtractPDFText returns the plain text embedded in a PDF. Best-effort: the underlying reader can panic
// on malformed input, so this recovers and returns an error instead of crashing the worker. An empty or
// text-less PDF returns a short/empty string (not an error) — the caller decides via SufficientText.
func ExtractPDFText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text = ""
			err = fmt.Errorf("ocr: pdf text extraction panicked: %v", r)
		}
	}()
	if len(data) == 0 {
		return "", fmt.Errorf("ocr: empty pdf")
	}
	reader, rerr := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if rerr != nil {
		return "", fmt.Errorf("ocr: read pdf: %w", rerr)
	}
	buf, terr := reader.GetPlainText()
	if terr != nil {
		return "", fmt.Errorf("ocr: extract pdf text: %w", terr)
	}
	out, ierr := io.ReadAll(buf)
	if ierr != nil {
		return "", fmt.Errorf("ocr: read pdf text: %w", ierr)
	}
	return strings.TrimSpace(string(out)), nil
}

// SufficientText decides whether tier-1 (local, non-AI) text is trustworthy enough to skip the AI
// fallback. A real receipt has some length AND digits (amounts/dates); a handful of stray glyphs from a
// mostly-image PDF should escalate to AI, not be persisted as a transcription.
func SufficientText(s string) bool {
	s = strings.TrimSpace(s)
	if len([]rune(s)) < minReceiptTextRunes {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= minReceiptDigits
}
