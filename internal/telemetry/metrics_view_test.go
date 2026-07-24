package telemetry

import (
	"context"
	"testing"

	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// A request duration recorded in SECONDS must land in seconds-scale buckets, not the SDK's default
// millisecond boundaries {0, 5, … 10000}. Without the view, every sub-second request collapses into the
// first default bucket and histogram_quantile(0.95, …) returns a bogus ~4.75s regardless of real speed —
// the exact false positive behind the Openb00ksHighLatency alert. This guards the fix.
func TestHTTPDurationView_UsesSecondsBuckets(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(httpDurationView()),
	)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	h, err := mp.Meter("test").Float64Histogram("http.server.request.duration", apimetric.WithUnit("s"))
	if err != nil {
		t.Fatalf("histogram: %v", err)
	}
	h.Record(context.Background(), 0.001) // 1ms — a typical fast request, recorded in seconds.

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var found bool
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
				found = true
				// Seconds scale: top boundary is 10, not the SDK default's 10000.
				if got := dp.Bounds[len(dp.Bounds)-1]; got != 10 {
					t.Fatalf("top bucket bound = %v, want 10 (seconds); default ms buckets top out at 10000", got)
				}
				// A 1ms request (0.001s) must fall in the first bucket (<= 0.005s), proving seconds scale.
				if dp.BucketCounts[0] != 1 {
					t.Fatalf("1ms sample did not land in the first (<=0.005s) bucket; counts = %v", dp.BucketCounts)
				}
			}
		}
	}
	if !found {
		t.Fatal("no http.server.request.duration histogram collected")
	}
}
