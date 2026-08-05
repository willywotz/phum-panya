package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/config"
	"phum-panya/internal/telemetry"
)

func TestRequestMetricsRecordsRequestCount(t *testing.T) {
	metrics, shutdown, err := telemetry.Setup(context.Background(), config.Config{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer shutdown(context.Background())

	mw, err := telemetry.RequestMetrics()
	if err != nil {
		t.Fatalf("RequestMetrics: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	rec := httptest.NewRecorder()
	metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "http_server_request_count") {
		t.Fatalf("/metrics missing http_server_request_count:\n%s", rec.Body.String())
	}
}
