package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeS3 stands in for an S3/R2 endpoint: it accepts any request and returns 200. Signatures aren't
// validated — the point is to exercise Put/Ready request construction without a real bucket.
func fakeS3(putPath *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && putPath != nil {
			*putPath = r.URL.Path
		}
		w.Header().Set("ETag", `"deadbeef"`)
		w.WriteHeader(http.StatusOK)
	}))
}

func newTestStore(t *testing.T, endpoint string) *S3Store {
	t.Helper()
	s, err := NewS3Store(S3Config{
		Bucket:          "receipts",
		Endpoint:        endpoint,
		Region:          "auto",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		ForcePathStyle:  true,
		PresignTTL:      10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	return s
}

func TestS3Store_PutReturnsDatePrefixedKey(t *testing.T) {
	var putPath string
	srv := fakeS3(&putPath)
	defer srv.Close()
	s := newTestStore(t, srv.URL)

	data := []byte("receipt-bytes")
	key, err := s.Put(context.Background(), "invoice.pdf", "application/pdf", int64(len(data)), byteReader{b: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasSuffix(key, "/invoice.pdf") {
		t.Fatalf("key %q missing name suffix", key)
	}
	if len(key) != len("20060102")+1+len("invoice.pdf") {
		t.Fatalf("unexpected key layout: %q", key)
	}
	if strings.ContainsRune(key, '\\') {
		t.Fatalf("S3 key must use forward slashes: %q", key)
	}
	if want := "/receipts/" + key; putPath != want {
		t.Fatalf("PUT path = %q, want %q", putPath, want)
	}
}

func TestS3Store_GetURLIsPresigned(t *testing.T) {
	s := newTestStore(t, "https://acc.r2.cloudflarestorage.com")
	url, err := s.GetURL(context.Background(), "20260719/invoice.pdf")
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	for _, want := range []string{"receipts/20260719/invoice.pdf", "X-Amz-Signature=", "X-Amz-Algorithm=", "X-Amz-Expires="} {
		if !strings.Contains(url, want) {
			t.Fatalf("presigned url missing %q:\n%s", want, url)
		}
	}
}

func TestS3Store_Ready(t *testing.T) {
	srv := fakeS3(nil)
	defer srv.Close()
	s := newTestStore(t, srv.URL)
	if err := s.Ready(context.Background()); err != nil {
		t.Fatalf("Ready: %v", err)
	}
}

func TestNewS3Store_Validation(t *testing.T) {
	cases := map[string]S3Config{
		"no bucket":     {Endpoint: "e", AccessKeyID: "a", SecretAccessKey: "s"},
		"no endpoint":   {Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"},
		"no access key": {Bucket: "b", Endpoint: "e", SecretAccessKey: "s"},
		"no secret":     {Bucket: "b", Endpoint: "e", AccessKeyID: "a"},
	}
	for name, c := range cases {
		if _, err := NewS3Store(c); err == nil {
			t.Fatalf("%s: expected validation error, got nil", name)
		}
	}
}
