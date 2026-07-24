package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

type byteReader struct {
	b *bytes.Reader
}

func (r byteReader) Read(p []byte) (int, error) {
	return r.b.Read(p)
}

func TestLocalStorePutAndURL(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStore(dir, "http://localhost/receipts")

	data := []byte("hello")
	key, err := store.Put(context.TODO(), "test.txt", "text/plain", int64(len(data)), byteReader{b: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}

	path := filepath.Join(dir, key)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist")
	}

	url, err := store.GetURL(context.TODO(), key)
	if err != nil {
		t.Fatalf("GetURL error: %v", err)
	}
	if url == key {
		t.Fatalf("expected url to be absolute")
	}
}
