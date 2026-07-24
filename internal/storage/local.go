package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

type LocalStore struct {
	baseDir string
	baseURL string
}

func NewLocalStore(baseDir, baseURL string) *LocalStore {
	return &LocalStore{baseDir: baseDir, baseURL: baseURL}
}

func (s *LocalStore) Put(ctx context.Context, name string, contentType string, size int64, r Reader) (string, error) {
	_ = ctx
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return "", err
	}
	key := filepath.Join(time.Now().UTC().Format("20060102"), name)
	path := filepath.Join(s.baseDir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, readerAdapter{r: r})
	if err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return key, nil
}

func (s *LocalStore) GetURL(ctx context.Context, key string) (string, error) {
	_ = ctx
	if s.baseURL == "" {
		return key, nil
	}
	return s.baseURL + "/" + key, nil
}

type readerAdapter struct {
	r Reader
}

func (r readerAdapter) Read(p []byte) (int, error) {
	return r.r.Read(p)
}
