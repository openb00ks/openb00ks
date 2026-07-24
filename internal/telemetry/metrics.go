package telemetry

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// httpDurationBuckets are explicit histogram boundaries, in SECONDS, for http.server.request.duration.
//
// WHY THIS EXISTS: the OTel SDK's DEFAULT histogram boundaries are {0, 5, 10, 25, … 10000}, which assume
// MILLISECONDS. We record request duration in seconds (WithUnit("s"), time.Since().Seconds()), so every
// real sub-second request collapses into the first default bucket — and histogram_quantile then reports a
// bogus latency (0.95 × 5 = 4.75 "seconds") regardless of actual speed. These boundaries match the OTel
// HTTP semantic-convention recommended seconds buckets so quantiles are meaningful.
var httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// httpDurationView overrides the default aggregation for the request-duration histogram with
// seconds-scale buckets (see httpDurationBuckets).
func httpDurationView() metric.View {
	return metric.NewView(
		metric.Instrument{Name: "http.server.request.duration"},
		metric.Stream{Aggregation: metric.AggregationExplicitBucketHistogram{Boundaries: httpDurationBuckets}},
	)
}

// SetupMetrics installs a global OpenTelemetry MeterProvider backed by a Prometheus exporter and
// starts Go runtime instrumentation (GC, goroutines, memory). Metrics are pull-based — the platform
// scrapes them — so unlike tracing this is always on and does not depend on an OTLP endpoint.
//
// It returns an http.Handler that renders the metrics in Prometheus text format and a shutdown func.
// Call it BEFORE opening the database so otelsql's DB-pool stats (registered in db.Open against the
// global meter) land in this provider.
func SetupMetrics(ctx context.Context, serviceName string) (http.Handler, func(context.Context) error, error) {
	reg := prometheus.NewRegistry()

	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, nil, err
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, nil, err
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithResource(res),
		metric.WithView(httpDurationView()),
	)
	otel.SetMeterProvider(mp)

	// Go runtime metrics via the same provider (exported alongside the app metrics).
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return nil, nil, err
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	return handler, mp.Shutdown, nil
}

// MetricsServer builds an HTTP server that exposes the Prometheus metrics handler at /metrics (plus a
// trivial /healthz liveness probe) on addr. It is a dedicated port so app internals are never served
// on the public API surface. Run ListenAndServe in a goroutine and Shutdown on exit.
func MetricsServer(addr string, handler http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
