package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/db"
	httpapi "github.com/openb00ks/openb00ks/internal/http"
	"github.com/openb00ks/openb00ks/internal/logging"
	"github.com/openb00ks/openb00ks/internal/migrate"
	"github.com/openb00ks/openb00ks/internal/queue"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/telemetry"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		runAdmin(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "search" {
		runSearch(os.Args[2:])
		return
	}
	cfg := config.Load()
	logging.Setup(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: false,
	})
	shutdown, err := telemetry.Setup(context.Background(), telemetry.FromEnv("openb00ks-api"))
	if err != nil {
		slog.Error("otel setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			slog.Error("otel shutdown failed", "err", err)
		}
	}()
	// Metrics (Prometheus, pull-based) on a dedicated port, kept off the public API surface. Set up
	// before the DB opens so otelsql's pool stats register against this meter provider.
	metricsHandler, metricsShutdown, err := telemetry.SetupMetrics(context.Background(), "openb00ks-api")
	if err != nil {
		slog.Error("otel metrics setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := metricsShutdown(context.Background()); err != nil {
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
	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET is required")
		os.Exit(1)
	}
	tokens, err := auth.NewTokenService(cfg.JWTSecret, time.Now)
	if err != nil {
		slog.Error("token service error", "err", err)
		os.Exit(1)
	}
	jwtTTL := time.Duration(cfg.JWTTTLSeconds) * time.Second
	refreshTTL := time.Duration(cfg.RefreshTTLSeconds) * time.Second

	manager := db.NewManager(cfg.DatabaseURL)

	var objects storage.ReceiptStore
	switch cfg.ReceiptStorage {
	case "s3":
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
		objects = s3store
	default:
		objects = storage.NewLocalStore(cfg.ReceiptLocalDir, "")
	}
	receiptCfg := httpapi.NewReceiptHandler(cfg.ReceiptMaxBytes)
	pricing := suggest.Pricing{
		InputCentsPer1KTokens:  cfg.AIInputCentsPer1KTokens,
		OutputCentsPer1KTokens: cfg.AIOutputCentsPer1KTokens,
	}
	systemInfo := httpapi.SystemInfo{
		AIProvider:                cfg.AIProvider,
		AIModel:                   cfg.OpenAIModel,
		ReceiptStorage:            cfg.ReceiptStorage,
		ReceiptLocalDir:           cfg.ReceiptLocalDir,
		ReceiptMaxBytes:           cfg.ReceiptMaxBytes,
		PublicRegistrationEnabled: cfg.EnablePublicRegistration,
	}
	hc := httpapi.NewHandlerContext(manager, tokens, jwtTTL, refreshTTL, cfg.CORSAllowedOrigins, pricing, objects, receiptCfg, systemInfo)
	searchProvider := searchpkg.OptionalFromConfig(cfg)
	if typesenseProvider, ok := searchProvider.(*searchpkg.TypesenseProvider); ok {
		if err := typesenseProvider.EnsureTransactionCollection(context.Background()); err != nil {
			slog.Warn("typesense transaction collection setup failed", "err", err)
		}
		if err := typesenseProvider.EnsureDocumentCollection(context.Background()); err != nil {
			slog.Warn("typesense document collection setup failed", "err", err)
		}
		if err := typesenseProvider.EnsureVendorCollection(context.Background()); err != nil {
			slog.Warn("typesense vendor collection setup failed", "err", err)
		}
	}
	hc.SetSearchProvider(searchProvider)
	server := httpapi.NewServer(hc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Start(ctx, func(conn *db.DB) {
		if err := migrate.Up(cfg.DatabaseURL, 0); err != nil {
			slog.Error("auto-migrate failed", "err", err)
			return
		}
		stores := db.NewStores(conn)
		if stores == nil {
			return
		}
		hc.SetStores(stores, queue.NewDBQueue(conn))
	})

	slog.Info("api listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server.Handler()); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func runMigrate(args []string) {
	cfg := config.Load()
	logging.Setup(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: false,
	})
	if len(args) == 0 {
		slog.Error("usage: openb00ks migrate <up|version|force>")
		os.Exit(1)
	}
	dbURL := os.Getenv("MIGRATE_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		slog.Error("DATABASE_URL or MIGRATE_DATABASE_URL is required")
		os.Exit(1)
	}
	switch args[0] {
	case "up":
		if err := migrate.Up(dbURL, 0); err != nil {
			slog.Error("migrate up failed", "err", err)
			os.Exit(1)
		}
	case "version":
		version, dirty, err := migrate.Version(dbURL)
		if err != nil {
			slog.Error("migrate version failed", "err", err)
			os.Exit(1)
		}
		status := "clean"
		if dirty {
			status = "dirty"
		}
		fmt.Printf("version=%d status=%s\n", version, status)
	case "force":
		if os.Getenv("MIGRATE_ALLOW_DANGEROUS") != "1" {
			slog.Error("force requires MIGRATE_ALLOW_DANGEROUS=1")
			os.Exit(1)
		}
		if len(args) < 2 {
			slog.Error("version is required")
			os.Exit(1)
		}
		version, err := strconv.Atoi(args[1])
		if err != nil {
			slog.Error("invalid version", "err", err)
			os.Exit(1)
		}
		if err := migrate.Force(dbURL, version); err != nil {
			slog.Error("migrate force failed", "err", err)
			os.Exit(1)
		}
	default:
		slog.Error("unknown migrate command")
		os.Exit(1)
	}
}
