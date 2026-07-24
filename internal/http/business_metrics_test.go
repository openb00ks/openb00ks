package httpapi

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestBusinessMetrics_CountsAndSourceLabel(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	m := newBusinessMetrics() // captures the manual-reader provider set above
	ctx := context.Background()
	m.transactionPosted(ctx, "direct")
	m.transactionPosted(ctx, "receipt")
	m.transactionPosted(ctx, "receipt")
	m.receiptUploaded(ctx)
	m.suggestionServed(ctx)
	m.suggestionServed(ctx)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	// name -> source-label (or "_" when absent) -> value
	got := map[string]map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, mm := range sm.Metrics {
			sum, ok := mm.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			got[mm.Name] = map[string]int64{}
			for _, dp := range sum.DataPoints {
				key := "_"
				if v, ok := dp.Attributes.Value(attribute.Key("source")); ok {
					key = v.AsString()
				}
				got[mm.Name][key] = dp.Value
			}
		}
	}

	if got["openb00ks.transactions.posted"]["direct"] != 1 {
		t.Fatalf("transactions.posted{source=direct} = %v, want 1", got["openb00ks.transactions.posted"])
	}
	if got["openb00ks.transactions.posted"]["receipt"] != 2 {
		t.Fatalf("transactions.posted{source=receipt} = %v, want 2", got["openb00ks.transactions.posted"])
	}
	if got["openb00ks.receipts.uploaded"]["_"] != 1 {
		t.Fatalf("receipts.uploaded = %v, want 1", got["openb00ks.receipts.uploaded"])
	}
	if got["openb00ks.suggestions.served"]["_"] != 2 {
		t.Fatalf("suggestions.served = %v, want 2", got["openb00ks.suggestions.served"])
	}
}

func TestBusinessMetrics_NilSafe(t *testing.T) {
	var m *businessMetrics
	// A HandlerContext without initialised metrics must not panic when handlers record events.
	m.transactionPosted(context.Background(), "direct")
	m.receiptUploaded(context.Background())
	m.suggestionServed(context.Background())
}
