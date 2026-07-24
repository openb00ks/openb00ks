package httpapi

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RequestMetrics records HTTP server metrics via the global OpenTelemetry meter, so they are exported
// on the Prometheus /metrics endpoint. It emits:
//
//   - http.server.request.duration — a histogram of request latency (seconds), labelled by method,
//     route template and status code. Request RATE and ERROR rate are derived from its _count series.
//   - http.server.active_requests — an in-flight gauge.
//
// The route label is the Gin route TEMPLATE (c.FullPath(), e.g. /entities/:id), never the raw path,
// so path parameters cannot explode metric cardinality; unmatched requests collapse to "unmatched".
// Naming follows OTEL HTTP semantic conventions, so the exporter renders
// http_server_request_duration_seconds_bucket{...}.
func RequestMetrics(service string) gin.HandlerFunc {
	meter := otel.Meter(service)
	duration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of inbound HTTP requests."),
	)
	inflight, _ := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of in-flight inbound HTTP requests."),
	)

	return func(c *gin.Context) {
		ctx := c.Request.Context()
		method := c.Request.Method
		methodAttr := metric.WithAttributes(attribute.String("http.request.method", method))

		inflight.Add(ctx, 1, methodAttr)
		start := time.Now()

		c.Next()

		inflight.Add(ctx, -1, methodAttr)

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", c.Writer.Status()),
		))
	}
}
