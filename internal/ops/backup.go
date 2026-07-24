package ops

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"time"
)

// Dumper writes a logical database dump to w (e.g. pg_dump). Abstracted so the task is testable
// without a real database.
type Dumper interface {
	Dump(ctx context.Context, w io.Writer) error
}

// Sink is the object store the backups land in (an R2/S3 bucket). Keys are full object keys.
type Sink interface {
	Put(ctx context.Context, key string, r io.ReadSeeker) error
	List(ctx context.Context, prefix string) ([]string, error) // returns keys, order unspecified
	Delete(ctx context.Context, key string) error
}

// BackupConfig configures the db-backup task.
type BackupConfig struct {
	Prefix    string        // key prefix, e.g. "backups"
	DBLabel   string        // logical name segment, e.g. "openbooks"
	Retention int           // keep this many most-recent dumps (<=0 keeps all)
	Interval  time.Duration // default cadence
	Enabled   bool          // default enabled
	now       func() time.Time
}

// NewBackupTask returns a scheduler Task that dumps the DB (gzip) to the sink under
// <prefix>/<db>/<UTC-timestamp>.sql.gz and prunes to Retention.
func NewBackupTask(dumper Dumper, sink Sink, cfg BackupConfig) Task {
	if cfg.Prefix == "" {
		cfg.Prefix = "backups"
	}
	if cfg.DBLabel == "" {
		cfg.DBLabel = "db"
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	dir := path.Join(cfg.Prefix, cfg.DBLabel)

	return Task{
		Name:            "db-backup",
		DefaultInterval: cfg.Interval,
		DefaultEnabled:  cfg.Enabled,
		Run: func(ctx context.Context) error {
			key := path.Join(dir, cfg.now().UTC().Format("20060102T150405Z")+".sql.gz")
			if err := dumpToSink(ctx, dumper, sink, key); err != nil {
				return err
			}
			return prune(ctx, sink, dir, cfg.Retention)
		},
	}
}

// dumpToSink streams dump -> gzip -> a temp file (seekable, so the upload needs no in-memory buffer and
// large dumps don't blow the heap) -> the sink.
func dumpToSink(ctx context.Context, dumper Dumper, sink Sink, key string) error {
	tmp, err := os.CreateTemp("", "obbackup-*.sql.gz")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	gz := gzip.NewWriter(tmp)
	if err := dumper.Dump(ctx, gz); err != nil {
		return fmt.Errorf("dump: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := sink.Put(ctx, key, tmp); err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

// prune deletes all but the newest `keep` objects under dir (keys are timestamp-suffixed, so lexical
// sort == chronological). keep <= 0 disables pruning.
func prune(ctx context.Context, sink Sink, dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	keys, err := sink.List(ctx, dir+"/")
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	if len(keys) <= keep {
		return nil
	}
	sort.Strings(keys)
	for _, key := range keys[:len(keys)-keep] {
		if err := sink.Delete(ctx, key); err != nil {
			return fmt.Errorf("prune %s: %w", key, err)
		}
	}
	return nil
}
