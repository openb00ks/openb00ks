package ocr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPDFAITranscriber_TranscribePDF(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  Total 12.34  "}}]}`))
	}))
	defer srv.Close()

	tr := NewPDFAITranscriber("test-key", "gpt-5-nano", 1024)
	tr.endpoint = srv.URL
	tr.http = srv.Client()

	res, err := tr.TranscribePDF(context.Background(), []byte("%PDF-1.4 pretend"))
	if err != nil {
		t.Fatalf("TranscribePDF: %v", err)
	}
	if res.Text != "Total 12.34" {
		t.Errorf("Text = %q, want trimmed %q", res.Text, "Total 12.34")
	}
	if res.Provider != "llm-vision-pdf" {
		t.Errorf("Provider = %q, want llm-vision-pdf", res.Provider)
	}
	if res.Model != "gpt-5-nano" {
		t.Errorf("Model = %q, want gpt-5-nano", res.Model)
	}

	// Request shape: gpt-5-compatible token param, and a file content part carrying a base64 PDF.
	if v, _ := gotBody["max_completion_tokens"].(float64); v != 1024 {
		t.Errorf("max_completion_tokens = %v, want 1024", gotBody["max_completion_tokens"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("no messages in request")
	}
	user, _ := msgs[len(msgs)-1].(map[string]any)
	parts, _ := user["content"].([]any)
	foundFile := false
	for _, p := range parts {
		pm, _ := p.(map[string]any)
		if pm["type"] != "file" {
			continue
		}
		foundFile = true
		file, _ := pm["file"].(map[string]any)
		fd, _ := file["file_data"].(string)
		if !strings.HasPrefix(fd, "data:application/pdf;base64,") {
			t.Errorf("file_data does not start with the pdf data-URL prefix: %.40q", fd)
		}
	}
	if !foundFile {
		t.Error("request had no file content part")
	}
}

func TestPDFAITranscriber_Errors(t *testing.T) {
	t.Parallel()

	if _, err := NewPDFAITranscriber("k", "m", 0).TranscribePDF(context.Background(), nil); err == nil {
		t.Error("expected error for empty pdf bytes")
	}
	if _, err := NewPDFAITranscriber("", "m", 0).TranscribePDF(context.Background(), []byte("x")); err == nil {
		t.Error("expected error for empty api key")
	}

	// An API error is surfaced (not swallowed) so the caller parks the receipt.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unsupported file"}}`))
	}))
	defer srv.Close()
	tr := NewPDFAITranscriber("k", "m", 0)
	tr.endpoint = srv.URL
	tr.http = srv.Client()
	_, err := tr.TranscribePDF(context.Background(), []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "unsupported file") {
		t.Errorf("want error containing 'unsupported file', got %v", err)
	}
}
