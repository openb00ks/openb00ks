package ops

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeDumper struct {
	data []byte
	err  error
}

func (d fakeDumper) Dump(_ context.Context, w io.Writer) error {
	if d.err != nil {
		return d.err
	}
	_, err := w.Write(d.data)
	return err
}

type memSink struct {
	objs map[string][]byte
}

func newMemSink() *memSink { return &memSink{objs: map[string][]byte{}} }

func (s *memSink) Put(_ context.Context, key string, r io.ReadSeeker) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.objs[key] = b
	return nil
}

func (s *memSink) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range s.objs {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *memSink) Delete(_ context.Context, key string) error {
	delete(s.objs, key)
	return nil
}

func TestBackupTask_UploadsGzippedDump(t *testing.T) {
	sink := newMemSink()
	task := NewBackupTask(fakeDumper{data: []byte("SQL DUMP")}, sink,
		BackupConfig{Prefix: "backups", DBLabel: "openbooks", Retention: 3})

	if err := task.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sink.objs) != 1 {
		t.Fatalf("want 1 object, got %d", len(sink.objs))
	}
	for key, gz := range sink.objs {
		if !strings.HasPrefix(key, "backups/openbooks/") || !strings.HasSuffix(key, ".sql.gz") {
			t.Fatalf("unexpected key: %q", key)
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			t.Fatalf("gzip: %v", err)
		}
		out, _ := io.ReadAll(zr)
		if string(out) != "SQL DUMP" {
			t.Fatalf("decoded %q, want %q", out, "SQL DUMP")
		}
	}
	if task.Name != "db-backup" {
		t.Fatalf("task name = %q", task.Name)
	}
}

func TestBackupTask_RetentionPrunesOldest(t *testing.T) {
	sink := newMemSink()
	cfg := BackupConfig{Prefix: "backups", DBLabel: "openbooks", Retention: 3}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n := 0
	cfg.now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Minute) } // distinct, increasing keys
	task := NewBackupTask(fakeDumper{data: []byte("x")}, sink, cfg)

	for i := 0; i < 6; i++ {
		if err := task.Run(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if len(sink.objs) != 3 {
		t.Fatalf("retention: want 3 kept, got %d", len(sink.objs))
	}
	// The kept keys must be the three newest (minutes 4,5,6).
	for k := range sink.objs {
		if strings.Contains(k, "000100Z") || strings.Contains(k, "000200Z") || strings.Contains(k, "000300Z") {
			t.Fatalf("pruned an old backup but it survived: %q", k)
		}
	}
}

func TestBackupTask_DumpErrorLeavesNoObject(t *testing.T) {
	sink := newMemSink()
	task := NewBackupTask(fakeDumper{err: errors.New("pg_dump boom")}, sink, BackupConfig{})
	if err := task.Run(context.Background()); err == nil {
		t.Fatal("expected dump error to propagate")
	}
	if len(sink.objs) != 0 {
		t.Fatalf("no object should be written on dump failure, got %d", len(sink.objs))
	}
}
