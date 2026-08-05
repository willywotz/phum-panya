package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"phum-panya/internal/config"
	"phum-panya/internal/telemetry"
)

func TestSetupMetricsHandlerServesPrometheus(t *testing.T) {
	// No OTLP endpoint: exporter is skipped, providers still build.
	h, shutdown, err := telemetry.Setup(context.Background(), config.Config{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer shutdown(context.Background())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	// Go runtime metrics are registered, so the body is non-empty Prometheus text.
	if rec.Body.Len() == 0 {
		t.Fatal("/metrics body empty")
	}
}
