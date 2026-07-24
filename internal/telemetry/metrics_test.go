package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupMetrics_ServesPrometheusText(t *testing.T) {
	handler, shutdown, err := SetupMetrics(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("SetupMetrics: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// The OTEL Prometheus exporter always emits target_info carrying the resource (service.name).
	if !strings.Contains(body, "target_info") {
		t.Fatalf("metrics output missing target_info; got:\n%s", head(body, 400))
	}
	if !strings.Contains(body, `service_name="test-service"`) {
		t.Fatalf("metrics output missing service_name label; got:\n%s", head(body, 400))
	}
}

func TestMetricsServer_HealthzAndMetrics(t *testing.T) {
	handler, shutdown, err := SetupMetrics(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("SetupMetrics: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	srv := MetricsServer(":0", handler)

	cases := []struct{ path, wantSubstr string }{
		{"/healthz", "ok"},
		{"/metrics", "# "}, // Prometheus HELP/TYPE comments
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), tc.wantSubstr) {
			t.Fatalf("%s: body missing %q", tc.path, tc.wantSubstr)
		}
	}
}

func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
