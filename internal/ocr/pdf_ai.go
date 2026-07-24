package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Tier-2 OCR for PDFs: when local text extraction (ExtractPDFText) comes up empty — a scanned or
// image-only PDF — escalate to a vision model by sending the PDF as a FILE input (base64). OpenAI reads
// the document directly (text + rendered pages); it rejects a PDF passed as image_url, which is exactly
// why the shared ai driver's Complete (image_url only) can't do this and we issue the call here.

// openAIChatURL is the chat-completions endpoint. A field on the transcriber overrides it in tests.
const openAIChatURL = "https://api.openai.com/v1/chat/completions"

// PDFAITranscriber transcribes a PDF via a vision-capable OpenAI model using a file content part.
type PDFAITranscriber struct {
	apiKey    string
	model     string
	maxTokens int
	endpoint  string
	http      *http.Client
}

// NewPDFAITranscriber builds the transcriber. maxTokens <= 0 defaults to 4096.
func NewPDFAITranscriber(apiKey, model string, maxTokens int) *PDFAITranscriber {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &PDFAITranscriber{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		endpoint:  openAIChatURL,
		http:      &http.Client{Timeout: 90 * time.Second},
	}
}

// Minimal subset of the OpenAI chat API, matching the shared driver's conventions (max_completion_tokens;
// a content array of typed parts). The "file" part is the piece the driver lacks.
type pdfChatRequest struct {
	Model       string       `json:"model"`
	Messages    []pdfMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_completion_tokens,omitempty"`
}

type pdfMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string (system) or []pdfContentPart (user)
}

type pdfContentPart struct {
	Type string      `json:"type"` // "text" | "file"
	Text string      `json:"text,omitempty"`
	File *pdfFileRef `json:"file,omitempty"`
}

type pdfFileRef struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"` // "data:application/pdf;base64,<...>"
}

type pdfChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// TranscribePDF returns the transcription of a PDF's bytes. It never fabricates: any API error is
// returned so the caller parks the receipt rather than persisting an empty/garbage transcription.
func (t *PDFAITranscriber) TranscribePDF(ctx context.Context, data []byte) (Result, error) {
	if len(data) == 0 {
		return Result{}, fmt.Errorf("ocr: empty pdf")
	}
	if t.apiKey == "" {
		return Result{}, fmt.Errorf("ocr: pdf-ai transcriber needs an OpenAI API key")
	}
	dataURL := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(data)
	reqBody := pdfChatRequest{
		Model:       t.model,
		Temperature: 0, // determinism — transcription must not vary
		MaxTokens:   t.maxTokens,
		Messages: []pdfMessage{
			{Role: "system", Content: transcribeSystem},
			{Role: "user", Content: []pdfContentPart{
				{Type: "text", Text: "Transcribe this receipt PDF."},
				{Type: "file", File: &pdfFileRef{Filename: "receipt.pdf", FileData: dataURL}},
			}},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return Result{}, fmt.Errorf("ocr: marshal pdf request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(raw))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ocr: pdf vision request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed pdfChatResponse
	if uerr := json.Unmarshal(respBody, &parsed); uerr != nil {
		return Result{}, fmt.Errorf("ocr: decode pdf response (status %d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if parsed.Error != nil {
			msg = parsed.Error.Message
		}
		return Result{}, fmt.Errorf("ocr: pdf vision failed (status %d): %s", resp.StatusCode, msg)
	}
	if len(parsed.Choices) == 0 {
		return Result{}, fmt.Errorf("ocr: pdf vision returned no choices")
	}
	return Result{
		Text:     strings.TrimSpace(parsed.Choices[0].Message.Content),
		Provider: "llm-vision-pdf",
		Model:    t.model,
	}, nil
}
