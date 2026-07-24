// Command ops-scheduler runs open-b00ks's recurring operational tasks (internal/ops). It is a small,
// self-contained scheduler backed by the app's own Postgres — no external cron. The first task is
// db-backup (pg_dump → gzip → R2); notifications, batch-AI enrichment, and cleanup register the same
// way. Built on a postgres:alpine base so a matching pg_dump is on PATH.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openb00ks/openb00ks/internal/aibatch"
	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/logging"
	"github.com/openb00ks/openb00ks/internal/ops"
	"github.com/openb00ks/openb00ks/internal/receiptbatch"
	"github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/telemetry"

	aipkg "github.com/spectrum-labs-tech/ai"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// otelMetrics reports task runs to the OTEL meter (scraped on the metrics port alongside the worker).
type otelMetrics struct {
	runs metric.Int64Counter
	dur  metric.Float64Histogram
}

func (m *otelMetrics) TaskRun(ctx context.Context, name, outcome string, d time.Duration) {
	attrs := metric.WithAttributes(attribute.String("task", name), attribute.String("outcome", outcome))
	m.runs.Add(ctx, 1, attrs)
	m.dur.Record(ctx, d.Seconds(), attrs)
}

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logging.Setup(logging.Config{Level: cfg.LogLevel, Format: cfg.LogFormat, AddSource: false})

	shutdown, err := telemetry.Setup(rootCtx, telemetry.FromEnv("openb00ks-ops-scheduler"))
	if err != nil {
		slog.Error("otel setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			slog.Error("otel shutdown failed", "err", err)
		}
	}()

	metricsHandler, metricsShutdown, err := telemetry.SetupMetrics(rootCtx, "openb00ks-ops-scheduler")
	if err != nil {
		slog.Error("otel metrics setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsShutdown(ctx); err != nil {
			slog.Error("otel metrics shutdown failed", "err", err)
		}
	}()
	metricsSrv := telemetry.MetricsServer(cfg.MetricsAddr, metricsHandler)
	go func() {
		slog.Info("metrics listening", "addr", cfg.MetricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server error", "err", err)
		}
	}()

	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	dbConn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database open error", "err", err)
		os.Exit(1)
	}
	defer func() { _ = dbConn.Close() }()

	meter := otel.Meter("openb00ks-ops-scheduler")
	runs, _ := meter.Int64Counter("openb00ks_ops_task_runs_total")
	dur, _ := meter.Float64Histogram("openb00ks_ops_task_duration_seconds")

	sched := ops.NewScheduler(db.NewScheduledTaskStore(dbConn), ops.Options{
		Tick:    time.Duration(cfg.SchedulerTickSeconds) * time.Second,
		Lease:   time.Duration(cfg.SchedulerLeaseSeconds) * time.Second,
		Metrics: &otelMetrics{runs: runs, dur: dur},
		Log:     slog.Default(),
	})

	registerTasks(cfg, sched, dbConn)

	slog.Info("ops-scheduler starting")
	if err := sched.Run(rootCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("scheduler stopped", "err", err)
		os.Exit(1)
	}
	slog.Info("ops-scheduler stopped")
}

// registerTasks wires the enabled ops tasks. Each capability self-gates on its config, so an
// unconfigured task simply isn't registered (nothing to break for a minimal self-host).
func registerTasks(cfg config.Config, sched *ops.Scheduler, dbConn *db.DB) {
	registerBackupTask(cfg, sched)
	registerBatchTasks(cfg, sched, dbConn)
	registerReindexTask(cfg, sched, dbConn)
}

// registerReindexTask periodically reconciles the search index — a full, idempotent reindex of every
// entity's documents (+ transactions) into Typesense. Index-on-write keeps search current in normal
// operation; this is the drift healer for documents that missed their write (search briefly down) or need
// re-shaping after a schema change (a search-reconcile pass sized for openb00ks: a full pass
// rather than a staleness queue, since the dataset is small). Registered only when Typesense is
// configured; SEARCH_RECONCILE_SECONDS=0 disables it.
func registerReindexTask(cfg config.Config, sched *ops.Scheduler, dbConn *db.DB) {
	if cfg.SearchReconcileSeconds <= 0 {
		return
	}
	provider := search.OptionalFromConfig(cfg)
	tp, ok := provider.(*search.TypesenseProvider)
	if !ok {
		slog.Info("search-reconcile task disabled (SEARCH_PROVIDER is not typesense)")
		return
	}
	reindexer := search.Reindexer{Provider: provider, Source: db.NewStores(dbConn).SearchSource}
	sched.Register(ops.Task{
		Name:            "search-reconcile",
		DefaultInterval: time.Duration(cfg.SearchReconcileSeconds) * time.Second,
		DefaultEnabled:  true,
		Timeout:         15 * time.Minute,
		Run: func(ctx context.Context) error {
			// Self-heal collection existence first (a fresh Typesense may have none yet).
			_ = tp.EnsureTransactionCollection(ctx)
			_ = tp.EnsureDocumentCollection(ctx)
			_ = tp.EnsureVendorCollection(ctx)
			txRes, err := reindexer.ReindexTransactions(ctx, search.ReindexOptions{})
			if err != nil {
				return err
			}
			docRes, err := reindexer.ReindexDocuments(ctx, search.ReindexOptions{})
			if err != nil {
				return err
			}
			slog.Info("search reconcile complete",
				"entities", docRes.EntityCount, "tx_indexed", txRes.IndexedCount,
				"doc_indexed", docRes.IndexedCount, "doc_failed", docRes.FailedCount, "vendors", docRes.VendorCount)
			return nil
		},
	})
	slog.Info("search-reconcile task registered", "interval_s", cfg.SearchReconcileSeconds)
}

func registerBackupTask(cfg config.Config, sched *ops.Scheduler) {
	if cfg.BackupS3Bucket == "" {
		slog.Info("db-backup task disabled (set BACKUP_S3_BUCKET to enable)")
		return
	}
	sink, err := ops.NewS3Sink(ops.S3SinkConfig{
		Bucket:          cfg.BackupS3Bucket,
		Endpoint:        cfg.BackupS3Endpoint,
		Region:          cfg.BackupS3Region,
		AccessKeyID:     cfg.BackupS3AccessKeyID,
		SecretAccessKey: cfg.BackupS3SecretAccessKey,
		ForcePathStyle:  cfg.BackupS3ForcePathStyle,
	})
	if err != nil {
		slog.Error("db-backup sink init failed", "err", err)
		os.Exit(1)
	}
	sched.Register(ops.NewBackupTask(
		ops.PgDumpDumper{DSN: cfg.DatabaseURL},
		sink,
		ops.BackupConfig{
			Prefix:    cfg.BackupS3Prefix,
			DBLabel:   cfg.BackupDBLabel,
			Retention: int(cfg.BackupRetention),
			Interval:  time.Duration(cfg.BackupIntervalSeconds) * time.Second,
			Enabled:   true,
		},
	))
	slog.Info("db-backup task registered", "bucket", cfg.BackupS3Bucket, "interval_s", cfg.BackupIntervalSeconds)
}

// registerBatchTasks wires the generic aibatch submit/collect/reset tasks with the receipt pipeline
// kinds, when PIPELINE_MODE=decomposed-batch. Batch uses the system OpenAI credentials (one batch
// spans many receipts/tenants, so it can't use per-tenant keys). Adding future batch operations means
// registering more kinds here — no new tasks.
func registerBatchTasks(cfg config.Config, sched *ops.Scheduler, dbConn *db.DB) {
	if cfg.PipelineMode != "decomposed-batch" {
		return
	}
	if cfg.OpenAIAPIKey == "" {
		slog.Warn("aibatch tasks disabled: OPENAI_API_KEY not set")
		return
	}
	provider, err := suggest.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	if err != nil {
		slog.Error("aibatch provider init failed", "err", err)
		os.Exit(1)
	}
	bp, ok := provider.(aipkg.BatchProvider)
	if !ok {
		slog.Error("aibatch: configured provider does not support the Batch API")
		os.Exit(1)
	}

	reg := aibatch.NewRegistry()
	for _, k := range receiptbatch.Kinds(receiptbatch.Deps{
		States:        db.NewReceiptPipelineStateStore(dbConn),
		Receipts:      db.NewReceiptStore(dbConn),
		OCR:           db.NewReceiptOCRStore(dbConn),
		Accounts:      db.NewAccountStore(dbConn),
		Drafts:        db.NewDraftStore(dbConn),
		Vendors:       db.NewVendorStore(dbConn),
		VendorAliases: db.NewVendorAliasStore(dbConn),
		Entities:      db.NewEntityStore(dbConn),
		Search:        search.OptionalFromConfig(cfg),
	}) {
		reg.Register(k)
	}
	store := db.NewAIBatchStore(dbConn)
	window := cfg.AIBatchWindow
	staleHours := cfg.AIBatchStaleHours

	sched.Register(ops.Task{
		Name: "aibatch-submit", DefaultInterval: time.Duration(cfg.AIBatchSubmitSeconds) * time.Second,
		DefaultEnabled: true, Timeout: 5 * time.Minute,
		Run: func(ctx context.Context) error { _, err := aibatch.SubmitAll(ctx, bp, store, reg, window); return err },
	})
	sched.Register(ops.Task{
		Name: "aibatch-collect", DefaultInterval: time.Duration(cfg.AIBatchCollectSeconds) * time.Second,
		DefaultEnabled: true, Timeout: 5 * time.Minute,
		Run: func(ctx context.Context) error { return aibatch.Collect(ctx, bp, store, reg) },
	})
	sched.Register(ops.Task{
		Name: "aibatch-reset-stuck", DefaultInterval: time.Hour, DefaultEnabled: true,
		Run: func(ctx context.Context) error {
			return aibatch.ResetStuck(ctx, store, reg, time.Now().Add(-time.Duration(staleHours)*time.Hour))
		},
	})
	slog.Info("aibatch receipt tasks registered", "kinds", len(reg.All()), "window", window)
}
