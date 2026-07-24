package storage

import "context"

type ReadyChecker interface {
	Ready(ctx context.Context) error
}

type ReceiptStore interface {
	Put(ctx context.Context, name string, contentType string, size int64, r Reader) (key string, err error)
	GetURL(ctx context.Context, key string) (string, error)
}

type Reader interface {
	Read(p []byte) (n int, err error)
}

type ReadCloser interface {
	Read(p []byte) (n int, err error)
	Close() error
}
