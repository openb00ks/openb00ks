package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace/noop"
)

type Config struct {
	ServiceName string
	Endpoint    string
	Protocol    string
	Insecure    bool
}

func FromEnv(defaultService string) Config {
	return Config{
		ServiceName: envOr("OTEL_SERVICE_NAME", defaultService),
		Endpoint:    strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		Protocol:    strings.ToLower(strings.TrimSpace(envOr("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"))),
		Insecure:    strings.ToLower(strings.TrimSpace(envOr("OTEL_EXPORTER_OTLP_INSECURE", "true"))) == "true",
	}
}

func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.Endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		slog.Info("otel tracing disabled (no exporter endpoint configured)")
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, err
	}

	exp, err := newExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("otel tracing enabled", "endpoint", cfg.Endpoint, "protocol", cfg.Protocol)
	return tp.Shutdown, nil
}

func newExporter(ctx context.Context, cfg Config) (trace.SpanExporter, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch cfg.Protocol {
	case "grpc":
		opts := []otlptracegrpc.Option{}
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(timeoutCtx, opts...)
	case "http/protobuf", "http":
		opts := []otlptracehttp.Option{}
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(timeoutCtx, opts...)
	default:
		return nil, errors.New("unsupported OTEL_EXPORTER_OTLP_PROTOCOL")
	}
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); strings.TrimSpace(val) != "" {
		return val
	}
	return fallback
}
