package httpapi

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func RequestTracing(service string) gin.HandlerFunc {
	tracer := otel.Tracer(service)
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		name := fmt.Sprintf("%s %s", c.Request.Method, path)
		ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer))
		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()

		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", path),
			attribute.String("http.target", c.Request.URL.Path),
			attribute.Int("http.status_code", c.Writer.Status()),
			attribute.Int("http.response_size", c.Writer.Size()),
			attribute.String("http.client_ip", c.ClientIP()),
		)
		span.SetAttributes(attribute.Int64("http.duration_ms", time.Since(start).Milliseconds()))
		if c.Writer.Status() >= 500 {
			span.SetStatus(codes.Error, "server_error")
		}
		span.End()
	}
}
