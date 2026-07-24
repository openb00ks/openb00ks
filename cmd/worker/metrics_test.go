package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecordJob_LabelsStageAndOutcome(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("test")
	dur, err := meter.Float64Histogram("openb00ks.worker.job.duration")
	if err != nil {
		t.Fatalf("histogram: %v", err)
	}
	count, err := meter.Int64Counter("openb00ks.worker.jobs.processed")
	if err != nil {
		t.Fatalf("counter: %v", err)
	}

	recordJob(context.Background(), dur, count, "suggest", 250*time.Millisecond, nil)        // success -> ack
	recordJob(context.Background(), dur, count, "", 10*time.Millisecond, errors.New("boom")) // "" -> ocr, fail

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "openb00ks.worker.jobs.processed" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("unexpected data type %T", m.Data)
			}
			for _, dp := range sum.DataPoints {
				stage, _ := dp.Attributes.Value(attribute.Key("stage"))
				outcome, _ := dp.Attributes.Value(attribute.Key("outcome"))
				got[stage.AsString()+"/"+outcome.AsString()] = dp.Value
			}
		}
	}

	if got["suggest/ack"] != 1 {
		t.Fatalf("suggest/ack = %d, want 1 (all: %v)", got["suggest/ack"], got)
	}
	if got["ocr/fail"] != 1 {
		t.Fatalf("ocr/fail = %d, want 1 — empty stage must normalise to ocr and error to fail (all: %v)", got["ocr/fail"], got)
	}
}
