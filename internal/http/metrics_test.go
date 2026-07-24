package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRequestMetrics_RecordsDurationWithRouteTemplate(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestMetrics("test"))
	r.GET("/entities/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Two requests to the SAME route with DIFFERENT ids must collapse to one route-template series.
	for _, id := range []string{"abc", "def"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/entities/"+id, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var count uint64
	var route, method string
	var status int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("unexpected data type %T", m.Data)
			}
			for _, dp := range hist.DataPoints {
				count = dp.Count
				if v, ok := dp.Attributes.Value(attribute.Key("http.route")); ok {
					route = v.AsString()
				}
				if v, ok := dp.Attributes.Value(attribute.Key("http.request.method")); ok {
					method = v.AsString()
				}
				if v, ok := dp.Attributes.Value(attribute.Key("http.response.status_code")); ok {
					status = v.AsInt64()
				}
			}
		}
	}

	if route != "/entities/:id" {
		t.Fatalf("route label = %q, want /entities/:id (raw paths must not explode cardinality)", route)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (both requests on one template series)", count)
	}
	if method != http.MethodGet || status != http.StatusOK {
		t.Fatalf("method/status = %q/%d, want GET/200", method, status)
	}
}

func TestRequestMetrics_UnmatchedRouteCollapses(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestMetrics("test"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such/path", nil))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist := m.Data.(metricdata.Histogram[float64])
			for _, dp := range hist.DataPoints {
				v, _ := dp.Attributes.Value(attribute.Key("http.route"))
				if v.AsString() != "unmatched" {
					t.Fatalf("route label = %q, want unmatched (raw 404 path must not be a label)", v.AsString())
				}
			}
		}
	}
}
