package db

import (
	"context"
	"errors"
	"strings"

	"log/slog"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

var ErrMissingDSN = errors.New("missing database dsn")

type DB struct {
	*sqlx.DB
}

func Open(dsn string) (*DB, error) {
	if dsn == "" {
		return nil, ErrMissingDSN
	}
	sqlDB, err := otelsql.Open("pgx", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, err
	}
	if _, err := otelsql.RegisterDBStatsMetrics(sqlDB, otelsql.WithAttributes(semconv.DBSystemPostgreSQL)); err != nil {
		slog.Warn("db stats metrics not registered", "err", err)
	}
	db := sqlx.NewDb(sqlDB, "pgx")
	db.MapperFunc(func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, "_", ""))
	})
	return &DB{DB: db}, nil
}

func (d *DB) Ready(ctx context.Context) error {
	if d == nil || d.DB == nil {
		return ErrMissingDSN
	}
	return d.PingContext(ctx)
}
