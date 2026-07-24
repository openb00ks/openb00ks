package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/logging"
	searchpkg "github.com/openb00ks/openb00ks/internal/search"
)

type reindexSearchOptions struct {
	TenantID string
	EntityID string
}

func runSearch(args []string) {
	cfg := config.Load()
	logging.Setup(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: false,
	})
	if len(args) == 0 {
		slog.Error("usage: openb00ks search reindex-transactions|reindex-documents [--tenant-id <id>] [--entity-id <id>]")
		os.Exit(1)
	}
	switch args[0] {
	case "reindex-transactions":
		opts, err := parseReindexSearchOptions(args[1:])
		if err != nil {
			slog.Error("invalid reindex-transactions options", "err", err)
			os.Exit(1)
		}
		result, err := reindexTransactions(cfg, opts)
		if err != nil {
			slog.Error("transaction reindex failed", "err", err)
			os.Exit(1)
		}
		fmt.Printf("reindexed transactions: entities=%d transactions=%d indexed=%d failed=%d\n", result.EntityCount, result.TransactionCount, result.IndexedCount, result.FailedCount)
	case "reindex-documents":
		opts, err := parseReindexSearchOptions(args[1:])
		if err != nil {
			slog.Error("invalid reindex-documents options", "err", err)
			os.Exit(1)
		}
		result, err := reindexDocuments(cfg, opts)
		if err != nil {
			slog.Error("document reindex failed", "err", err)
			os.Exit(1)
		}
		fmt.Printf("reindexed documents: entities=%d accounts=%d transactions=%d receipts=%d statements=%d mileage=%d documents=%d indexed=%d failed=%d\n", result.EntityCount, result.AccountCount, result.TransactionCount, result.ReceiptCount, result.StatementCount, result.MileageCount, result.DocumentCount, result.IndexedCount, result.FailedCount)
	default:
		slog.Error("unknown search command")
		os.Exit(1)
	}
}

func parseReindexSearchOptions(args []string) (reindexSearchOptions, error) {
	fs := flag.NewFlagSet("reindex-transactions", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := reindexSearchOptions{}
	fs.StringVar(&opts.TenantID, "tenant-id", "", "tenant id to reindex")
	fs.StringVar(&opts.EntityID, "entity-id", "", "entity id to reindex")
	if err := fs.Parse(args); err != nil {
		return reindexSearchOptions{}, err
	}
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.EntityID = strings.TrimSpace(opts.EntityID)
	return opts, nil
}

func reindexTransactions(cfg config.Config, opts reindexSearchOptions) (searchpkg.ReindexResult, error) {
	return reindexSearch(cfg, opts, false)
}

func reindexDocuments(cfg config.Config, opts reindexSearchOptions) (searchpkg.ReindexResult, error) {
	return reindexSearch(cfg, opts, true)
}

func reindexSearch(cfg config.Config, opts reindexSearchOptions, documents bool) (searchpkg.ReindexResult, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return searchpkg.ReindexResult{}, db.ErrMissingDSN
	}
	provider, err := searchpkg.NewFromConfig(cfg)
	if err != nil {
		return searchpkg.ReindexResult{}, err
	}
	if typesenseProvider, ok := provider.(*searchpkg.TypesenseProvider); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := typesenseProvider.EnsureTransactionCollection(ctx); err != nil {
			return searchpkg.ReindexResult{}, err
		}
		if documents {
			if err := typesenseProvider.EnsureDocumentCollection(ctx); err != nil {
				return searchpkg.ReindexResult{}, err
			}
			if err := typesenseProvider.EnsureVendorCollection(ctx); err != nil {
				return searchpkg.ReindexResult{}, err
			}
		}
	}
	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return searchpkg.ReindexResult{}, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("database close failed", "err", err)
		}
	}()
	stores := db.NewStores(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	reindexer := searchpkg.Reindexer{
		Provider: provider,
		Source:   stores.SearchSource,
	}
	options := searchpkg.ReindexOptions{
		TenantID: opts.TenantID,
		EntityID: opts.EntityID,
	}
	if documents {
		return reindexer.ReindexDocuments(ctx, options)
	}
	return reindexer.ReindexTransactions(ctx, options)
}
