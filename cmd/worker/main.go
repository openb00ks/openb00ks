package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openb00ks/openb00ks/internal/aiconfig"
	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/importer"
	"github.com/openb00ks/openb00ks/internal/logging"
	"github.com/openb00ks/openb00ks/internal/models"
	ocrpkg "github.com/openb00ks/openb00ks/internal/ocr"
	"github.com/openb00ks/openb00ks/internal/pipeline"
	"github.com/openb00ks/openb00ks/internal/queue"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/telemetry"
	"github.com/openb00ks/openb00ks/internal/vendormemo"
	aipkg "github.com/spectrum-labs-tech/ai"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logging.Setup(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: false,
	})
	shutdown, err := telemetry.Setup(rootCtx, telemetry.FromEnv("openb00ks-worker"))
	if err != nil {
		slog.Error("otel setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			slog.Error("otel shutdown failed", "err", err)
		}
	}()
	// Metrics (Prometheus, pull-based) on a dedicated port. Set up before the DB opens so otelsql's
	// pool stats register against this meter provider; the queue job metrics below use it too.
	metricsHandler, metricsShutdown, err := telemetry.SetupMetrics(rootCtx, "openb00ks-worker")
	if err != nil {
		slog.Error("otel metrics setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsShutdown(shutdownCtx); err != nil {
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

	q := queue.NewDBQueue(dbConn)
	receipts := db.NewReceiptStore(dbConn)
	entities := db.NewEntityStore(dbConn)
	drafts := db.NewDraftStore(dbConn)
	importRows := db.NewImportRowStore(dbConn)
	rules := db.NewVendorRuleStore(dbConn)
	vendors := db.NewVendorStore(dbConn)
	vendorAliases := db.NewVendorAliasStore(dbConn)
	accounts := db.NewAccountStore(dbConn)
	ocr := db.NewReceiptOCRStore(dbConn)
	suggestions := db.NewReceiptSuggestionStore(dbConn)
	receiptMeta := db.NewReceiptMetadataStore(dbConn)
	processingErrors := db.NewProcessingErrorStore(dbConn)
	objects := buildReceiptStore(cfg)
	pipelineStates := db.NewReceiptPipelineStateStore(dbConn)
	pricing := suggest.Pricing{
		InputCentsPer1KTokens:  cfg.AIInputCentsPer1KTokens,
		OutputCentsPer1KTokens: cfg.AIOutputCentsPer1KTokens,
	}
	aiResolver := aiconfig.NewResolver(cfg)
	searcher := searchpkg.OptionalFromConfig(cfg)
	w := &worker{
		receipts:         receipts,
		entities:         entities,
		receiptMeta:      receiptMeta,
		drafts:           drafts,
		importRows:       importRows,
		rules:            rules,
		vendors:          vendors,
		vendorAliases:    vendorAliases,
		accounts:         accounts,
		ocr:              ocr,
		suggestions:      suggestions,
		processingErrors: processingErrors,
		q:                q,
		pricing:          pricing,
		aiResolver:       aiResolver,
		searcher:         searcher,
		objects:          objects,
		cfg:              cfg,
		pipelineStates:   pipelineStates,
	}
	if typesenseProvider, ok := searcher.(*searchpkg.TypesenseProvider); ok {
		if err := typesenseProvider.EnsureTransactionCollection(rootCtx); err != nil {
			slog.Warn("typesense collection setup failed", "err", err)
		}
		if err := typesenseProvider.EnsureVendorCollection(rootCtx); err != nil {
			slog.Warn("typesense vendor collection setup failed", "err", err)
		}
	}

	workerID := envOr("WORKER_ID", "worker-1")
	batchSize := envOrInt("QUEUE_BATCH", 5)
	lockSeconds := envOrInt("QUEUE_LOCK_SECONDS", 300)
	maxAttempts := envOrInt("QUEUE_MAX_ATTEMPTS", 5)

	slog.Info("worker started", "id", workerID, "batch", batchSize, "lock_seconds", lockSeconds)

	meter := otel.Meter("openb00ks-worker")
	jobDuration, err := meter.Float64Histogram(
		"openb00ks.worker.job.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of processing a single queue job, by stage and outcome."),
	)
	if err != nil {
		slog.Warn("job duration metric unavailable", "err", err)
	}
	jobsProcessed, err := meter.Int64Counter(
		"openb00ks.worker.jobs.processed",
		metric.WithDescription("Queue jobs processed, by stage and outcome (ack|fail)."),
	)
	if err != nil {
		slog.Warn("jobs processed metric unavailable", "err", err)
	}

	for {
		if rootCtx.Err() != nil {
			slog.Info("worker stopping", "reason", rootCtx.Err())
			return
		}

		claimCtx, cancelClaim := context.WithTimeout(rootCtx, 30*time.Second)
		jobs, err := q.Claim(claimCtx, queue.ClaimRequest{
			WorkerID:    workerID,
			BatchSize:   batchSize,
			LockSeconds: lockSeconds,
			MaxAttempts: maxAttempts,
		})
		cancelClaim()
		if err != nil {
			if rootCtx.Err() != nil {
				slog.Info("worker stopping", "reason", rootCtx.Err())
				return
			}
			slog.Error("queue claim error", "err", err)
			if !sleepWithContext(rootCtx, 2*time.Second) {
				return
			}
			continue
		}
		if len(jobs) == 0 {
			if !sleepWithContext(rootCtx, 2*time.Second) {
				return
			}
			continue
		}

		for _, job := range jobs {
			if rootCtx.Err() != nil {
				slog.Info("worker stopping", "reason", rootCtx.Err())
				return
			}

			jobCtx, cancelJob := context.WithTimeout(rootCtx, 45*time.Second)
			jobStart := time.Now()
			err := w.processJob(jobCtx, job)
			cancelJob()
			recordJob(rootCtx, jobDuration, jobsProcessed, string(job.Stage), time.Since(jobStart), err)

			ackCtx, cancelAck := context.WithTimeout(rootCtx, 10*time.Second)
			if err != nil {
				slog.Error("job failed", "job_id", job.ID, "err", err)
				if failErr := q.Fail(ackCtx, job.ID, err.Error(), true); failErr != nil {
					slog.Error("queue fail record failed", "job_id", job.ID, "err", failErr)
				}
				cancelAck()
				continue
			}
			if ackErr := q.Ack(ackCtx, job.ID); ackErr != nil {
				slog.Error("queue ack failed", "job_id", job.ID, "err", ackErr)
			}
			cancelAck()
		}
	}
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// recordJob emits the per-job duration histogram and processed counter, labelled by stage and
// outcome (ack on success, fail on error). An empty stage is normalised to the OCR default so the
// labels line up with the processJob switch.
func recordJob(ctx context.Context, dur metric.Float64Histogram, count metric.Int64Counter, stage string, elapsed time.Duration, jobErr error) {
	if stage == "" {
		stage = string(queue.StageOCR)
	}
	outcome := "ack"
	if jobErr != nil {
		outcome = "fail"
	}
	attrs := metric.WithAttributes(
		attribute.String("stage", stage),
		attribute.String("outcome", outcome),
	)
	if dur != nil {
		dur.Record(ctx, elapsed.Seconds(), attrs)
	}
	if count != nil {
		count.Add(ctx, 1, attrs)
	}
}

// worker bundles the stores, config, and resolvers each job stage needs so the
// stage functions are methods rather than 15-to-20-parameter free functions.
type worker struct {
	receipts         *db.ReceiptStore
	entities         *db.EntityStore
	receiptMeta      *db.ReceiptMetadataStore
	drafts           *db.DraftStore
	importRows       *db.ImportRowStore
	rules            *db.VendorRuleStore
	vendors          *db.VendorStore
	vendorAliases    *db.VendorAliasStore
	accounts         *db.AccountStore
	ocr              *db.ReceiptOCRStore
	suggestions      *db.ReceiptSuggestionStore
	processingErrors *db.ProcessingErrorStore
	q                *queue.DBQueue
	pricing          suggest.Pricing
	aiResolver       *aiconfig.Resolver
	searcher         searchpkg.Provider
	objects          storage.ReceiptStore
	cfg              config.Config
	pipelineStates   *db.ReceiptPipelineStateStore
}

func (w *worker) processJob(ctx context.Context, job queue.Job) error {
	if w.receipts == nil || w.drafts == nil {
		return queue.ErrQueueUnavailable
	}
	switch job.Stage {
	case queue.StageOCR, "":
		return w.runOCR(ctx, job.ReceiptID)
	case queue.StageSuggest:
		if w.cfg.PipelineMode == "decomposed-batch" {
			// Hand off to the async batch pipeline: the ops-scheduler's aibatch tasks take it from
			// 'extract' onward (receipt_pipeline_state). Terminal for the worker.
			return w.pipelineStates.Ensure(ctx, job.ReceiptID)
		}
		if w.cfg.PipelineMode == "decomposed" {
			return w.runPipeline(ctx, job.ReceiptID)
		}
		return w.runSuggest(ctx, job.ReceiptID)
	case queue.StageDraft:
		return w.runDraft(ctx, job.ReceiptID)
	default:
		return nil
	}
}

// runOCR is pipeline stage 1: transcribe the receipt image to text (or empty when OCR is disabled),
// persist it as a receipt_ocr row, and hand off to the suggest stage. Transcription is intentionally
// separate from field extraction (a later stage) — one narrow AI ask, easier to validate.
func (w *worker) runOCR(ctx context.Context, receiptID string) error {
	receipts, entities, ocr := w.receipts, w.entities, w.ocr
	processingErrors, q, objects := w.processingErrors, w.q, w.objects
	aiResolver, cfg := w.aiResolver, w.cfg
	if err := receipts.UpdateStatus(ctx, receiptID, "processing"); err != nil {
		return err
	}
	receipt, err := receipts.GetByID(ctx, receiptID)
	if err != nil {
		return err
	}

	// Tiered OCR: a PDF tries local (non-AI) text extraction first and only escalates to the vision
	// model when that text is missing or too thin (a scanned PDF); images go straight to the vision
	// model. Empty text (nothing configured) parks the receipt for manual entry rather than inventing
	// content.
	res, terr := transcribeReceipt(ctx, cfg, entities, aiResolver, receipt, objects)
	if terr != nil {
		recordOCRError(ctx, processingErrors, receipts, receipt, terr)
		return terr
	}

	if ocr != nil {
		runVersion := 1
		if latest, lerr := ocr.LatestByReceiptID(ctx, receiptID); lerr == nil {
			runVersion = latest.RunVersion + 1
		}
		provider := res.Provider
		if provider == "" {
			provider = "none"
		}
		if _, cerr := ocr.Create(ctx, models.ReceiptOCR{
			ReceiptID:  receipt.ID,
			Provider:   provider,
			Status:     "succeeded",
			RawText:    res.Text,
			DataJSON:   []byte(`{}`),
			RunVersion: runVersion,
		}); cerr != nil {
			recordOCRError(ctx, processingErrors, receipts, receipt, cerr)
			return cerr
		}
	}

	if q != nil {
		if _, enqErr := q.Enqueue(ctx, queue.EnqueueRequest{ReceiptID: receiptID, Stage: queue.StageSuggest}); enqErr != nil {
			slog.Error("failed to enqueue suggest stage", "receipt_id", receiptID, "err", enqErr)
		}
	}
	return nil
}

// ocrTranscriber builds the transcriber for a receipt. The returned value may hold an AI provider; if
// it implements io.Closer the caller must Close it (runOCR does). Falls back to Noop on any gap.
func ocrTranscriber(ctx context.Context, cfg config.Config, entities *db.EntityStore, aiResolver *aiconfig.Resolver, receipt models.Receipt) ocrpkg.Transcriber {
	if cfg.OCRProvider != "llm-vision" {
		return ocrpkg.Noop{}
	}
	aiCfg := resolveReceiptAI(ctx, entities, aiResolver, receipt)
	if !aiCfg.IsAIAvailable() {
		return ocrpkg.Noop{}
	}
	model := cfg.OCRModel
	if model == "" {
		model = aiCfg.Model
	}
	provider, perr := suggest.NewOpenAIProvider(aiCfg.APIKey, model)
	if perr != nil {
		slog.Warn("ocr provider init failed; using noop", "receipt_id", receipt.ID, "err", perr)
		return ocrpkg.Noop{}
	}
	return &closingTranscriber{Transcriber: ocrpkg.NewLLMVision(provider, int(cfg.OCRMaxTokens)), provider: provider}
}

// resolveReceiptAI resolves the system/tenant AI config for a receipt (the tenant comes from its entity).
func resolveReceiptAI(ctx context.Context, entities *db.EntityStore, aiResolver *aiconfig.Resolver, receipt models.Receipt) aiconfig.AIConfig {
	var tenantID string
	if entities != nil {
		if entity, gerr := entities.GetByID(ctx, receipt.EntityID); gerr == nil {
			tenantID = entity.TenantID
		}
	}
	if aiResolver == nil {
		return aiconfig.AIConfig{}
	}
	return aiResolver.Resolve(ctx, tenantID)
}

// isPDF reports whether the content type is a PDF (case-insensitive; tolerant of parameters).
func isPDF(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "pdf")
}

// transcribeReceipt runs the tiered OCR for a receipt: a PDF goes through transcribePDF (local text
// first, then AI); an image goes straight to the vision model. It owns the vision provider's lifecycle.
func transcribeReceipt(ctx context.Context, cfg config.Config, entities *db.EntityStore, aiResolver *aiconfig.Resolver, receipt models.Receipt, objects storage.ReceiptStore) (ocrpkg.Result, error) {
	if isPDF(receipt.ContentType) {
		return transcribePDF(ctx, cfg, entities, aiResolver, receipt, objects)
	}
	transcriber := ocrTranscriber(ctx, cfg, entities, aiResolver, receipt)
	if closer, ok := transcriber.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	imageURL := ""
	if objects != nil {
		if u, uerr := objects.GetURL(ctx, receipt.StorageKey); uerr == nil {
			imageURL = u
		}
	}
	return transcriber.Transcribe(ctx, imageURL, receipt.ContentType)
}

// transcribePDF is the PDF OCR tier ladder: tier 1 = local text extraction (no AI); escalate to tier 2 =
// AI file-input OCR only when the local text is empty or too thin (a scanned/image PDF). Returns empty
// text (parked upstream) when there is no local text and no AI is configured.
func transcribePDF(ctx context.Context, cfg config.Config, entities *db.EntityStore, aiResolver *aiconfig.Resolver, receipt models.Receipt, objects storage.ReceiptStore) (ocrpkg.Result, error) {
	data, err := fetchReceiptBytes(ctx, objects, receipt.StorageKey, cfg.ReceiptMaxBytes)
	if err != nil {
		return ocrpkg.Result{}, err
	}
	// Tier 1: local, non-AI text extraction — free and deterministic for text-layer PDFs.
	if text, terr := ocrpkg.ExtractPDFText(data); terr != nil {
		slog.Info("pdf local text extraction failed; considering AI escalation", "receipt_id", receipt.ID, "err", terr)
	} else if ocrpkg.SufficientText(text) {
		return ocrpkg.Result{Text: text, Provider: "pdf-text"}, nil
	} else {
		slog.Info("pdf local text too thin; escalating to AI OCR", "receipt_id", receipt.ID, "chars", len(text))
	}
	// Tier 2: AI file-input OCR — only when llm-vision is enabled and available.
	if cfg.OCRProvider != "llm-vision" {
		return ocrpkg.Result{Provider: "none"}, nil
	}
	aiCfg := resolveReceiptAI(ctx, entities, aiResolver, receipt)
	if !aiCfg.IsAIAvailable() {
		return ocrpkg.Result{Provider: "none"}, nil
	}
	model := cfg.OCRModel
	if model == "" {
		model = aiCfg.Model
	}
	return ocrpkg.NewPDFAITranscriber(aiCfg.APIKey, model, int(cfg.OCRMaxTokens)).TranscribePDF(ctx, data)
}

// fetchReceiptBytes downloads a stored object via its (presigned) URL, capped at maxBytes. PDF OCR needs
// the bytes locally — for text extraction and for the AI file input — unlike image OCR, where the model
// fetches the URL itself.
func fetchReceiptBytes(ctx context.Context, objects storage.ReceiptStore, key string, maxBytes int64) ([]byte, error) {
	if objects == nil {
		return nil, fmt.Errorf("ocr: no object store configured")
	}
	url, err := objects.GetURL(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("ocr: presign receipt: %w", err)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("ocr: pdf OCR needs a fetchable URL (set RECEIPT_STORAGE=s3); got %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ocr: fetch receipt: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ocr: fetch receipt: status %d", resp.StatusCode)
	}
	limit := maxBytes
	if limit <= 0 {
		limit = 10 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("ocr: read receipt: %w", err)
	}
	return data, nil
}

// closingTranscriber ties an AI provider's lifecycle to the transcriber so runOCR can Close it.
type closingTranscriber struct {
	ocrpkg.Transcriber
	provider aipkg.Provider
}

func (c *closingTranscriber) Close() error { return c.provider.Close() }

func recordOCRError(ctx context.Context, processingErrors *db.ProcessingErrorStore, receipts *db.ReceiptStore, receipt models.Receipt, cause error) {
	if processingErrors != nil {
		if _, peErr := processingErrors.Create(ctx, models.ProcessingError{
			EntityID:  receipt.EntityID,
			ReceiptID: receipt.ID,
			Stage:     "ocr",
			Error:     cause.Error(),
		}); peErr != nil {
			slog.Warn("failed to record processing error", "receipt_id", receipt.ID, "stage", "ocr", "err", peErr)
		}
	}
	if statusErr := receipts.UpdateStatus(ctx, receipt.ID, "needs_attention"); statusErr != nil {
		slog.Error("failed to update receipt status", "receipt_id", receipt.ID, "status", "needs_attention", "err", statusErr)
	}
}

// buildReceiptStore mirrors the API's receipt-store selection so the worker can resolve a fetchable
// image URL for OCR. Local storage keys are not fetchable by a vision model (llm-vision needs S3).
func buildReceiptStore(cfg config.Config) storage.ReceiptStore {
	if cfg.ReceiptStorage == "s3" {
		s3store, err := storage.NewS3Store(storage.S3Config{
			Bucket:          cfg.ReceiptS3Bucket,
			Endpoint:        cfg.ReceiptS3Endpoint,
			Region:          cfg.ReceiptS3Region,
			AccessKeyID:     cfg.ReceiptS3AccessKeyID,
			SecretAccessKey: cfg.ReceiptS3SecretAccessKey,
			ForcePathStyle:  cfg.ReceiptS3ForcePathStyle,
			PresignTTL:      time.Duration(cfg.ReceiptS3PresignTTLSecs) * time.Second,
		})
		if err != nil {
			slog.Error("receipt storage (s3) init failed", "err", err)
			os.Exit(1)
		}
		return s3store
	}
	return storage.NewLocalStore(cfg.ReceiptLocalDir, "")
}

// aiCompleter adapts the shared ai.Provider to pipeline.Completer, pinning temperature 0 so every
// pipeline stage is deterministic.
type aiCompleter struct{ p aipkg.Provider }

func (a aiCompleter) Complete(ctx context.Context, system, user, schema string) (string, error) {
	zero := 0.0
	return a.p.Complete(ctx, system, user, schema, aipkg.Options{Temperature: &zero})
}

// runPipeline is the decomposed suggest stage (PIPELINE_MODE=decomposed): run the staged pipeline over
// the OCR text and, if every stage gates clean, write a balanced draft for human approval. Anything
// low-confidence or invalid parks the receipt at needs_attention with a processing_errors row — it is
// never auto-posted. This stage is terminal (it produces the draft directly; no separate draft stage).
func (w *worker) runPipeline(ctx context.Context, receiptID string) error {
	receipts, entities, ocrStore := w.receipts, w.entities, w.ocr
	drafts, accounts, vendors := w.drafts, w.accounts, w.vendors
	vendorAliases, searcher, processingErrors := w.vendorAliases, w.searcher, w.processingErrors
	aiResolver := w.aiResolver
	receipt, err := receipts.GetByID(ctx, receiptID)
	if err != nil {
		return err
	}

	ocrText := ""
	if ocrStore != nil {
		if latest, lerr := ocrStore.LatestByReceiptID(ctx, receiptID); lerr == nil {
			ocrText = latest.RawText
		}
	}
	if strings.TrimSpace(ocrText) == "" {
		pipelinePark(ctx, receipts, processingErrors, receipt, "extract", "no OCR text (transcription empty or OCR disabled)")
		return nil
	}

	var tenantID string
	if entities != nil {
		if e, gerr := entities.GetByID(ctx, receipt.EntityID); gerr == nil {
			tenantID = e.TenantID
		}
	}
	var aiCfg aiconfig.AIConfig
	if aiResolver != nil {
		aiCfg = aiResolver.Resolve(ctx, tenantID)
	}
	if !aiCfg.IsAIAvailable() {
		pipelinePark(ctx, receipts, processingErrors, receipt, "extract", "AI provider not configured")
		return nil
	}
	provider, perr := suggest.NewOpenAIProvider(aiCfg.APIKey, aiCfg.Model)
	if perr != nil {
		return perr
	}
	defer func() { _ = provider.Close() }()

	// Candidate accounts (the classifier picks one by ID) + the funding account for the credit leg.
	var acctRefs []pipeline.AccountRef
	if accounts != nil {
		if list, aerr := accounts.ListForEntity(ctx, receipt.EntityID, 500); aerr == nil {
			for _, a := range list {
				acctRefs = append(acctRefs, pipeline.AccountRef{Code: a.ID, Name: a.Name, Type: a.Type})
			}
		}
	}
	creditAccount := ""
	if accounts != nil {
		if cash, cerr := accounts.FindDefaultCashAccount(ctx, receipt.EntityID); cerr == nil {
			creditAccount = cash.ID
		}
	}

	memo := vendormemo.Deps{Vendors: vendors, Aliases: vendorAliases, Search: searcher}

	// Candidates: retrieve + rank the entity's known vendors against the extracted vendor name, so a
	// receipt from a previously-seen vendor MATCHES (and reuses its default account) instead of being
	// re-proposed. The pipeline calls this with the extracted name after the extract stage.
	candidates := func(vendorName string) []pipeline.VendorRef {
		return memo.Candidates(ctx, tenantID, receipt.EntityID, vendorName)
	}

	res, rerr := pipeline.Run(ctx, aiCompleter{p: provider}, pipeline.RunInput{
		OCRText:       ocrText,
		Accounts:      acctRefs,
		CreditAccount: creditAccount,
		Candidates:    candidates,
	})
	if rerr != nil {
		return rerr // transport error → the queue retries
	}
	if res.Status != "ready" || res.Entry == nil {
		pipelinePark(ctx, receipts, processingErrors, receipt, res.FailedStage, strings.Join(res.Issues, "; "))
		return nil
	}

	entries := make([]models.DraftEntry, 0, len(res.Entry.Lines))
	for _, l := range res.Entry.Lines {
		entries = append(entries, models.DraftEntry{AccountID: l.AccountCode, DebitCents: l.DebitCents, CreditCents: l.CreditCents})
	}
	date := time.Now().UTC()
	if res.Entry.Date != "" {
		if d, derr := time.Parse("2006-01-02", res.Entry.Date); derr == nil {
			date = d
		}
	}
	if drafts != nil {
		if _, eerr := drafts.EnsureForReceipt(ctx, receiptID); eerr != nil {
			return eerr
		}
		if _, uerr := drafts.UpdateDraft(ctx, receiptID, date, res.Entry.Memo, entries); uerr != nil {
			pipelinePark(ctx, receipts, processingErrors, receipt, "build-entry", uerr.Error())
			return uerr
		}
	}
	if serr := receipts.UpdateStatus(ctx, receiptID, "ready_for_review"); serr != nil {
		slog.Error("failed to set receipt status", "receipt_id", receiptID, "status", "ready_for_review", "err", serr)
	}

	// When the AI recommended a NEW vendor (none matched), create it as a first-class vendor with the
	// classified account as its default. This memoizes the vendor so future receipts MATCH it (via the
	// Candidates retrieval above) and reuse the account — one AI vendor call per novel vendor. Create
	// upserts on (entity, normalized name), so it's idempotent.
	rawVendor := ""
	if res.Extract.VendorName != nil {
		rawVendor = strings.TrimSpace(*res.Extract.VendorName)
	}
	resolvedVendorID := ""
	switch {
	case res.ProposedVendor != nil && res.Classify != nil:
		if v, ok := memo.Memoize(ctx, tenantID, receipt.EntityID, res.ProposedVendor, res.Classify.AccountCode); ok {
			memo.RecordResolution(ctx, tenantID, receipt.EntityID, v.ID, rawVendor)
			resolvedVendorID = v.ID
			slog.Info("memoized vendor from AI proposal", "receipt_id", receiptID, "vendor", res.ProposedVendor.Name)
		}
	case res.VendorID != nil:
		// Matched an existing vendor — learn this receipt's raw string as another alias.
		memo.RecordResolution(ctx, tenantID, receipt.EntityID, *res.VendorID, rawVendor)
		resolvedVendorID = *res.VendorID
	}
	// Persist the resolved vendor + raw string so posting the draft can feed the reviewer's account choice
	// back to this vendor (the feedback loop).
	if resolvedVendorID != "" {
		if err := receipts.SetResolvedVendor(ctx, receiptID, resolvedVendorID, rawVendor); err != nil {
			slog.Warn("failed to persist resolved vendor", "receipt_id", receiptID, "err", err)
		}
	}
	// Persist a display summary so the review UI can explain the suggestion (vendor + account, with
	// confidence + reason) before the human approves.
	if summary := buildAISummary(ctx, res, resolvedVendorID, vendors); summary.HasContent() {
		if err := receipts.SetAISummary(ctx, receiptID, &summary); err != nil {
			slog.Warn("failed to persist ai summary", "receipt_id", receiptID, "err", err)
		}
	}
	return nil
}

// buildAISummary distills a pipeline result into the compact, display-oriented summary the review UI
// shows. The matched vendor's clean name comes from the DB; a proposed vendor's from the result itself.
func buildAISummary(ctx context.Context, res pipeline.RunResult, resolvedVendorID string, vendors *db.VendorStore) models.ReceiptAISummary {
	var s models.ReceiptAISummary
	switch {
	case res.ProposedVendor != nil:
		s.Vendor = &models.AIVendorSummary{Name: res.ProposedVendor.Name, IsNew: true}
	case resolvedVendorID != "":
		name := ""
		if vendors != nil {
			if v, err := vendors.GetByID(ctx, resolvedVendorID); err == nil {
				name = v.Name
			}
		}
		s.Vendor = &models.AIVendorSummary{Name: name}
	}
	if s.Vendor != nil {
		if res.VendorMatch != nil {
			s.Vendor.Confidence = res.VendorMatch.Confidence
			s.Vendor.Reason = res.VendorMatch.Reason
		} else {
			// No AI vendor call was made — a deterministic exact-name match.
			s.Vendor.Confidence = 1
			s.Vendor.Reason = "exact name match"
		}
	}
	if res.Classify != nil {
		s.Account = &models.AIAccountSummary{
			AccountID:  res.Classify.AccountCode,
			Confidence: res.Classify.Confidence,
			Reason:     res.Classify.Reason,
		}
	}
	return s
}

// pipelinePark records why a receipt stopped and moves it to manual review (never auto-posted).
func pipelinePark(ctx context.Context, receipts *db.ReceiptStore, processingErrors *db.ProcessingErrorStore, receipt models.Receipt, stage, msg string) {
	if processingErrors != nil {
		if _, peErr := processingErrors.Create(ctx, models.ProcessingError{
			EntityID:  receipt.EntityID,
			ReceiptID: receipt.ID,
			Stage:     stage,
			Error:     msg,
		}); peErr != nil {
			slog.Warn("failed to record processing error", "receipt_id", receipt.ID, "stage", stage, "err", peErr)
		}
	}
	if statusErr := receipts.UpdateStatus(ctx, receipt.ID, "needs_attention"); statusErr != nil {
		slog.Error("failed to update receipt status", "receipt_id", receipt.ID, "status", "needs_attention", "err", statusErr)
	}
}

// suggestInputs is the entity/account/receipt context the legacy suggest stage
// feeds to its rule and AI matching.
type suggestInputs struct {
	entityContext string
	accountRows   []models.Account
	roleHints     []string
	itemContext   string
	extractedText string
}

// buildSuggestInputs gathers that context from the stores (all reads, no writes).
func (w *worker) buildSuggestInputs(ctx context.Context, receipt models.Receipt) suggestInputs {
	entityContext := ""
	accountRows := []models.Account{}
	if w.entities != nil {
		if entity, err := w.entities.GetByID(ctx, receipt.EntityID); err == nil {
			entityContext = entity.SuggestionContext
		}
	}
	roleHints := []string{}
	if w.accounts != nil {
		if rows, err := w.accounts.ListForEntity(ctx, receipt.EntityID, 1000); err == nil {
			accountRows = rows
			roleHints = accountRoleHintTokens(rows)
		}
	}
	if len(roleHints) > 0 {
		if entityContext != "" {
			entityContext += " "
		}
		entityContext += "Account role tags: " + strings.Join(roleHints, ", ")
	}
	itemContext := ""
	if w.receiptMeta != nil {
		if ctxText, err := w.receiptMeta.GetSuggestionContext(ctx, receipt.ID); err == nil {
			itemContext = ctxText
		}
	}
	extractedText := ""
	if w.ocr != nil {
		if latest, err := w.ocr.LatestByReceiptID(ctx, receipt.ID); err == nil {
			extractedText = latest.RawText
		}
	}
	return suggestInputs{entityContext, accountRows, roleHints, itemContext, extractedText}
}

func (w *worker) runSuggest(ctx context.Context, receiptID string) error {
	receipts, entities := w.receipts, w.entities
	drafts, importRows, rules := w.drafts, w.importRows, w.rules
	suggestions := w.suggestions
	processingErrors, q, pricing := w.processingErrors, w.q, w.pricing
	aiResolver, searcher := w.aiResolver, w.searcher
	receipt, err := receipts.GetByID(ctx, receiptID)
	if err != nil {
		return err
	}
	if receipt.Status != "processing" {
		if statusErr := receipts.UpdateStatus(ctx, receiptID, "processing"); statusErr != nil {
			slog.Warn("failed to set receipt status to processing", "receipt_id", receiptID, "err", statusErr)
		}
	}
	if receipt.Kind != "import" {
		if _, err := drafts.EnsureForReceipt(ctx, receiptID); err != nil {
			return err
		}
	}

	var tenantID string
	if entities != nil {
		if entity, err := entities.GetByID(ctx, receipt.EntityID); err == nil {
			tenantID = entity.TenantID
		}
	}

	var aiCfg aiconfig.AIConfig
	if aiResolver != nil {
		aiCfg = aiResolver.Resolve(ctx, tenantID)
	}

	var match *models.VendorRule
	if suggestions != nil {
		in := w.buildSuggestInputs(ctx, receipt)
		entityContext := in.entityContext
		accountRows := in.accountRows
		roleHints := in.roleHints
		itemContext := in.itemContext
		extractedText := in.extractedText

		importResult := importer.ParseCSV(extractedText)
		importSummary := importResult.Summary
		historicalCandidates := []searchpkg.Candidate{}
		if searcher != nil {
			candidateQuery := suggestionSearchQuery(receipt.OriginalName, itemContext, extractedText, importSummary.TopVendor, roleHints)
			if candidateQuery != "" {
				if rows, err := searcher.SuggestCandidates(ctx, searchpkg.CandidateQuery{
					TenantID:    tenantID,
					EntityID:    receipt.EntityID,
					Query:       candidateQuery,
					AmountCents: receipt.TotalCents,
					Limit:       5,
				}); err == nil {
					historicalCandidates = rows
				} else {
					slog.Warn("historical search failed", "receipt_id", receipt.ID, "err", err)
				}
			}
		}

		if rules != nil {
			candidates := ruleMatchCandidates(receipt.OriginalName, itemContext, extractedText, importSummary.TopVendor, strings.Join(roleHints, " "))
			for _, candidate := range candidates {
				matches, err := rules.FindMatching(ctx, receipt.EntityID, candidate)
				if err != nil {
					return err
				}
				if len(matches) > 0 {
					match = &matches[0]
					break
				}
			}
		}

		prompt := map[string]interface{}{
			"entity_id":          receipt.EntityID,
			"total_cents":        receipt.TotalCents,
			"original_name":      receipt.OriginalName,
			"receipt_kind":       receipt.Kind,
			"is_bulk":            receipt.Kind == "import",
			"entity_context":     entityContext,
			"suggestion_context": itemContext,
			"extracted_text":     extractedText,
			"account_role_tags":  roleHints,
			"historical_matches": historicalCandidates,
		}
		promptJSON, _ := json.Marshal(prompt)
		parsed := map[string]interface{}{}
		confidence := 0.0
		if match != nil {
			parsed["account_id"] = match.AccountID
			confidence = 0.6
		}
		if match == nil {
			if candidate, ok := searchpkg.BestCandidate(historicalCandidates, 0.85); ok {
				parsed["account_id"] = candidate.AccountID
				parsed["historical_match_transaction_id"] = candidate.TransactionID
				parsed["historical_match_score"] = candidate.Score
				parsed["explanation"] = "Matched a high-confidence accepted transaction from search history."
				confidence = 0.72
			}
		}
		if match == nil && confidence < 0.7 && receipt.Kind != "import" && aiCfg.IsAIAvailable() {
			if provider, err := suggest.NewOpenAIProvider(aiCfg.APIKey, aiCfg.Model); err == nil {
				aiConfidence, aiParsed, err := completeReceiptSuggestion(ctx, provider, receipt, entityContext, itemContext, extractedText, roleHints, accountRows, historicalCandidates)
				_ = provider.Close()
				if err == nil {
					for key, value := range aiParsed {
						if _, exists := parsed[key]; !exists {
							parsed[key] = value
						}
					}
					if aiConfidence > confidence {
						confidence = aiConfidence
					}
				}
			}
		}
		if receipt.Kind == "import" {
			importRowHints := make([]map[string]interface{}, 0, len(importResult.Rows))
			storedImportRows := make([]models.ImportRow, 0, len(importResult.Rows))
			accountCounts := map[string]int{}
			for _, row := range importResult.Rows {
				hint := map[string]interface{}{"row_index": row.RowIndex}
				accountID := ""
				if row.Date != "" {
					hint["date"] = row.Date
				}
				if row.Vendor != "" {
					hint["vendor"] = row.Vendor
				}
				if row.AmountCents > 0 {
					hint["amount_cents"] = row.AmountCents
				}
				if row.Direction != "" {
					hint["direction"] = row.Direction
				}
				if row.Memo != "" {
					hint["memo"] = row.Memo
				}
				if row.Fingerprint != "" {
					hint["fingerprint"] = row.Fingerprint
				}
				if rules != nil && row.Vendor != "" {
					matches, err := rules.FindMatching(ctx, receipt.EntityID, row.Vendor)
					if err != nil {
						return err
					}
					if len(matches) > 0 {
						accountID = matches[0].AccountID
						hint["account_id"] = accountID
						hint["rule_match_type"] = matches[0].MatchType
						hint["rule_pattern"] = matches[0].Pattern
						accountCounts[accountID]++
					}
				}
				importRowHints = append(importRowHints, hint)
				if parsedDate, err := time.Parse("2006-01-02", row.Date); err == nil {
					rawJSON, _ := json.Marshal(row.Raw)
					status := "needs_review"
					if accountID != "" {
						status = "mapped"
					}
					storedImportRows = append(storedImportRows, models.ImportRow{
						ReceiptID:   receipt.ID,
						EntityID:    receipt.EntityID,
						RowIndex:    row.RowIndex,
						Date:        parsedDate,
						Vendor:      row.Vendor,
						Memo:        row.Memo,
						AmountCents: row.AmountCents,
						Direction:   string(row.Direction),
						AccountID:   accountID,
						Fingerprint: row.Fingerprint,
						Status:      status,
						RawJSON:     rawJSON,
					})
				}
			}
			if importRows != nil && len(storedImportRows) > 0 {
				if err := importRows.ReplaceForReceipt(ctx, receipt.ID, receipt.EntityID, storedImportRows); err != nil {
					return err
				}
			}

			if match == nil {
				if dominantAccountID := dominantAccount(accountCounts); dominantAccountID != "" {
					parsed["account_id"] = dominantAccountID
					confidence = 0.65
				}
			}

			parsed["import_summary"] = importSummary
			if len(importRowHints) > 0 {
				parsed["import_rows"] = importRowHints
			}
			if len(importResult.Errors) > 0 {
				parsed["import_errors"] = importResult.Errors
			}
			if importSummary.TotalCents > 0 {
				parsed["total_cents"] = importSummary.TotalCents
			}
			if importSummary.TopVendor != "" {
				parsed["vendor_hint"] = importSummary.TopVendor
			}
			if match != nil {
				confidence = 0.7
			}
		}
		parsedJSON, _ := json.Marshal(parsed)

		provider := aiCfg.Provider
		suggestionModel := "rules"
		status := models.SuggestionStatusSucceeded

		if aiCfg.IsAIAvailable() {
			suggestionModel = aiCfg.Model
			if suggestionModel == "" {
				suggestionModel = "rules"
			}
		} else if aiCfg.LimitExceeded {
			status = models.SuggestionStatusLimitExceeded
		}

		rawJSON, _ := json.Marshal(map[string]interface{}{
			"provider":               provider,
			"ai_source":              string(aiCfg.Source),
			"ai_enabled":             aiCfg.Available,
			"historical_match_count": len(historicalCandidates),
			"status":                 "ok",
		})
		promptTokens := estimateTokens(promptJSON)
		completionTokens := estimateTokens(parsedJSON)
		costCents := suggest.EstimateCostCents(promptTokens, completionTokens, pricing)
		runVersion := 1
		if latest, err := suggestions.LatestByReceiptID(ctx, receiptID); err == nil {
			runVersion = latest.RunVersion + 1
		}
		_, err := suggestions.Create(ctx, models.ReceiptSuggestion{
			ReceiptID:        receiptID,
			Provider:         provider,
			Model:            suggestionModel,
			Status:           status,
			PromptJSON:       promptJSON,
			RawJSON:          rawJSON,
			ParsedJSON:       parsedJSON,
			Confidence:       confidence,
			RunVersion:       runVersion,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			CostCents:        costCents,
		})
		if err != nil {
			if processingErrors != nil {
				if _, peErr := processingErrors.Create(ctx, models.ProcessingError{
					EntityID:  receipt.EntityID,
					ReceiptID: receipt.ID,
					Stage:     "suggest",
					Error:     err.Error(),
				}); peErr != nil {
					slog.Warn("failed to record processing error", "receipt_id", receipt.ID, "stage", "suggest", "err", peErr)
				}
			}
			if statusErr := receipts.UpdateStatus(ctx, receipt.ID, "needs_attention"); statusErr != nil {
				slog.Error("failed to update receipt status", "receipt_id", receipt.ID, "status", "needs_attention", "err", statusErr)
			}
			return err
		}
	}
	if q != nil {
		if _, enqErr := q.Enqueue(ctx, queue.EnqueueRequest{ReceiptID: receiptID, Stage: queue.StageDraft}); enqErr != nil {
			slog.Error("failed to enqueue draft stage", "receipt_id", receiptID, "err", enqErr)
		}
	}
	return nil
}

func completeReceiptSuggestion(ctx context.Context, provider aipkg.Provider, receipt models.Receipt, entityContext, itemContext, extractedText string, roleHints []string, accountRows []models.Account, historicalCandidates []searchpkg.Candidate) (float64, map[string]interface{}, error) {
	systemPrompt := "You classify bookkeeping receipts. Return only JSON that matches the schema."
	if entityContext != "" {
		systemPrompt += "\nEntity context: " + entityContext
	}
	if len(roleHints) > 0 {
		systemPrompt += "\nRelevant account role tags: " + strings.Join(roleHints, ", ")
	}
	accountContext := accountPromptContext(accountRows)
	if accountContext != "" {
		systemPrompt += "\nChart of accounts:\n" + accountContext
	}
	if historicalContext := historicalCandidatePromptContext(historicalCandidates); historicalContext != "" {
		systemPrompt += "\nAccepted historical matches:\n" + historicalContext
	}
	userPrompt := strings.TrimSpace(strings.Join([]string{
		"Receipt ID: " + receipt.ID,
		"Original name: " + receipt.OriginalName,
		"Receipt kind: " + receipt.Kind,
		"Total cents: " + strconv.FormatInt(receipt.TotalCents, 10),
		"Suggestion context: " + itemContext,
		"Extracted text: " + extractedText,
	}, "\n"))
	schema := `{
		"type":"object",
		"properties":{
			"account_id":{"type":"string"},
			"explanation":{"type":"string"},
			"confidence":{"type":"number"}
		},
		"required":["account_id","explanation"],
		"additionalProperties":false
	}`
	raw, err := provider.Complete(ctx, systemPrompt, userPrompt, schema, aipkg.Options{})
	if err != nil {
		return 0, nil, err
	}
	parsed := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return 0, nil, err
	}
	confidence := 0.55
	if v, ok := parsed["confidence"].(float64); ok && v > 0 {
		confidence = v
	}
	accountID, ok := parsed["account_id"].(string)
	if !ok || strings.TrimSpace(accountID) == "" {
		return 0, nil, nil
	}
	if len(accountRows) > 0 {
		allowed := map[string]struct{}{}
		for _, account := range accountRows {
			allowed[account.ID] = struct{}{}
		}
		if _, ok := allowed[strings.TrimSpace(accountID)]; !ok {
			return 0, nil, nil
		}
	}
	return confidence, parsed, nil
}

func historicalCandidatePromptContext(candidates []searchpkg.Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	rows := make([]string, 0, len(candidates))
	for idx, candidate := range candidates {
		if idx >= 5 {
			break
		}
		row := candidate.TransactionID + " | account=" + candidate.AccountID
		if candidate.AccountName != "" {
			row += " " + candidate.AccountName
		}
		if candidate.Memo != "" {
			row += " | memo=" + candidate.Memo
		}
		if candidate.AmountCents > 0 {
			row += " | amount_cents=" + strconv.FormatInt(candidate.AmountCents, 10)
		}
		if candidate.Score > 0 {
			row += " | score=" + strconv.FormatFloat(candidate.Score, 'f', 2, 64)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

func suggestionSearchQuery(originalName, itemContext, extractedText, topVendor string, roleHints []string) string {
	extractedText = strings.Join(strings.Fields(extractedText), " ")
	if len(extractedText) > 500 {
		extractedText = extractedText[:500]
	}
	return searchpkg.NormalizeQuery(originalName, itemContext, topVendor, strings.Join(roleHints, " "), extractedText)
}

func accountPromptContext(accounts []models.Account) string {
	if len(accounts) == 0 {
		return ""
	}
	rows := make([]string, 0, len(accounts))
	for _, account := range accounts {
		label := account.ID + " | " + account.Name + " | " + account.Type
		if len(account.RoleTags) > 0 {
			label += " | tags=" + strings.Join(account.RoleTags, ",")
		}
		rows = append(rows, label)
	}
	sort.Strings(rows)
	if len(rows) > 20 {
		rows = rows[:20]
	}
	return strings.Join(rows, "\n")
}

func ruleMatchCandidates(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func accountRoleHintTokens(accounts []models.Account) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(accounts))
	for _, account := range accounts {
		for _, tag := range account.RoleTags {
			hint := strings.TrimSpace(tag)
			if hint == "" {
				continue
			}
			if _, ok := seen[hint]; ok {
				continue
			}
			seen[hint] = struct{}{}
			out = append(out, hint)
		}
	}
	sort.Strings(out)
	return out
}

func dominantAccount(counts map[string]int) string {
	bestID := ""
	bestCount := 0
	for accountID, count := range counts {
		if count > bestCount || (count == bestCount && bestID != "" && accountID < bestID) {
			bestID = accountID
			bestCount = count
		}
	}
	return bestID
}

func estimateTokens(payload []byte) int64 {
	if len(payload) == 0 {
		return 0
	}
	return int64(len(payload)) / 4
}

func (w *worker) runDraft(ctx context.Context, receiptID string) error {
	receipts, drafts, accounts := w.receipts, w.drafts, w.accounts
	suggestions, processingErrors := w.suggestions, w.processingErrors
	receipt, err := receipts.GetByID(ctx, receiptID)
	if err != nil {
		return err
	}
	draft, err := drafts.EnsureForReceipt(ctx, receiptID)
	if err != nil {
		return err
	}
	if len(draft.Entries) > 0 {
		return receipts.UpdateStatus(ctx, receiptID, "ready_for_review")
	}
	var entries []models.DraftEntry
	if suggestions != nil {
		if latest, err := suggestions.LatestByReceiptID(ctx, receiptID); err == nil {
			defaultCash := ""
			if accounts != nil {
				if cash, err := accounts.FindDefaultCashAccount(ctx, receipt.EntityID); err == nil {
					defaultCash = cash.ID
				}
			}
			entries = buildEntriesFromSuggestion(receipt, latest, defaultCash)
		}
	}
	if len(entries) > 0 {
		if err := drafts.SetEntriesByReceipt(ctx, receiptID, entries); err != nil {
			if processingErrors != nil {
				if _, peErr := processingErrors.Create(ctx, models.ProcessingError{
					EntityID:  receipt.EntityID,
					ReceiptID: receiptID,
					Stage:     "draft",
					Error:     err.Error(),
				}); peErr != nil {
					slog.Warn("failed to record processing error", "receipt_id", receiptID, "stage", "draft", "err", peErr)
				}
			}
			if statusErr := receipts.UpdateStatus(ctx, receiptID, "needs_attention"); statusErr != nil {
				slog.Error("failed to update receipt status", "receipt_id", receiptID, "status", "needs_attention", "err", statusErr)
			}
			return err
		}
	}
	if err := receipts.UpdateStatus(ctx, receiptID, "ready_for_review"); err != nil {
		if processingErrors != nil {
			if _, peErr := processingErrors.Create(ctx, models.ProcessingError{
				EntityID:  receipt.EntityID,
				ReceiptID: receiptID,
				Stage:     "draft",
				Error:     err.Error(),
			}); peErr != nil {
				slog.Warn("failed to record processing error", "receipt_id", receiptID, "stage", "draft", "err", peErr)
			}
		}
		return err
	}
	return nil
}

func buildEntriesFromSuggestion(receipt models.Receipt, suggestion models.ReceiptSuggestion, defaultCashAccountID string) []models.DraftEntry {
	if len(suggestion.ParsedJSON) == 0 {
		return nil
	}
	var payload map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(string(suggestion.ParsedJSON)))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil
	}

	if entries := parseEntries(payload); len(entries) > 0 {
		return entries
	}

	if receipt.Kind == "import" {
		if importEntries := buildImportEntriesFromSuggestion(payload, defaultCashAccountID); len(importEntries) > 0 {
			return importEntries
		}
	}

	total := parseInt64(payload, "total_cents")
	if total <= 0 {
		total = receipt.TotalCents
	}
	if total <= 0 {
		return nil
	}

	accountID := parseString(payload, "account_id")
	if accountID == "" {
		return nil
	}
	creditAccountID := parseString(payload, "payment_account_id")
	if creditAccountID == "" {
		creditAccountID = parseString(payload, "credit_account_id")
	}
	if creditAccountID == "" {
		creditAccountID = defaultCashAccountID
	}

	entries := []models.DraftEntry{
		{
			AccountID:   accountID,
			DebitCents:  total,
			CreditCents: 0,
		},
	}
	if creditAccountID != "" {
		entries = append(entries, models.DraftEntry{
			AccountID:   creditAccountID,
			DebitCents:  0,
			CreditCents: total,
		})
	}
	return entries
}

func buildImportEntriesFromSuggestion(payload map[string]interface{}, defaultCashAccountID string) []models.DraftEntry {
	rowsRaw, ok := payload["import_rows"]
	if !ok {
		return nil
	}
	rows, ok := rowsRaw.([]interface{})
	if !ok || len(rows) == 0 {
		return nil
	}

	fallbackAccountID := parseString(payload, "account_id")
	debitByAccount := map[string]int64{}
	creditByAccount := map[string]int64{}
	total := int64(0)
	cashDebits := int64(0)
	cashCredits := int64(0)

	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		amount := parseInt64(row, "amount_cents")
		if amount <= 0 {
			continue
		}
		accountID := parseString(row, "account_id")
		if accountID == "" {
			accountID = fallbackAccountID
		}
		if accountID == "" {
			continue
		}
		switch parseString(row, "direction") {
		case string(importer.DirectionInflow):
			creditByAccount[accountID] += amount
			cashDebits += amount
		default:
			debitByAccount[accountID] += amount
			cashCredits += amount
		}
		total += amount
	}

	if total <= 0 || (len(debitByAccount) == 0 && len(creditByAccount) == 0) {
		return nil
	}

	creditAccountID := parseString(payload, "payment_account_id")
	if creditAccountID == "" {
		creditAccountID = parseString(payload, "credit_account_id")
	}
	if creditAccountID == "" {
		creditAccountID = defaultCashAccountID
	}
	if creditAccountID == "" {
		return nil
	}

	accountIDs := make([]string, 0, len(debitByAccount)+len(creditByAccount))
	for accountID := range debitByAccount {
		accountIDs = append(accountIDs, accountID)
	}
	for accountID := range creditByAccount {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Strings(accountIDs)

	entries := make([]models.DraftEntry, 0, len(accountIDs)+2)
	if cashDebits > 0 {
		entries = append(entries, models.DraftEntry{
			AccountID:   creditAccountID,
			DebitCents:  cashDebits,
			CreditCents: 0,
		})
	}
	for _, accountID := range accountIDs {
		if amount := debitByAccount[accountID]; amount > 0 {
			entries = append(entries, models.DraftEntry{
				AccountID:   accountID,
				DebitCents:  amount,
				CreditCents: 0,
			})
		}
		if amount := creditByAccount[accountID]; amount > 0 {
			entries = append(entries, models.DraftEntry{
				AccountID:   accountID,
				DebitCents:  0,
				CreditCents: amount,
			})
		}
	}
	if cashCredits > 0 {
		entries = append(entries, models.DraftEntry{
			AccountID:   creditAccountID,
			DebitCents:  0,
			CreditCents: cashCredits,
		})
	}

	return entries
}

func parseEntries(payload map[string]interface{}) []models.DraftEntry {
	raw, ok := payload["entries"]
	if !ok {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	entries := make([]models.DraftEntry, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		accountID := parseString(row, "account_id")
		if accountID == "" {
			continue
		}
		debit := parseInt64(row, "debit_cents")
		credit := parseInt64(row, "credit_cents")
		if debit == 0 && credit == 0 {
			continue
		}
		entries = append(entries, models.DraftEntry{
			AccountID:   accountID,
			DebitCents:  debit,
			CreditCents: credit,
		})
	}
	return entries
}

func parseString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func parseInt64(payload map[string]interface{}, key string) int64 {
	v, ok := payload[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case json.Number:
		if n, err := val.Int64(); err == nil {
			return n
		}
	case float64:
		return int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
