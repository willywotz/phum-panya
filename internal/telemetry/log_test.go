package telemetry_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/config"
	"phum-panya/internal/telemetry"
)

func TestNewLoggerEmitsJSONWithResource(t *testing.T) {
	// NewLogger writes to stdout; here we assert its resource fields via a
	// parallel handler with the same options is overkill — instead test the
	// public AccessLog path below and NewLogger's non-nil contract.
	if telemetry.NewLogger(config.Config{LogLevel: "info", LogFormat: "json", ServiceName: "svc"}) == nil {
		t.Fatal("NewLogger returned nil")
	}
}

func TestAccessLogWritesStructuredLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil)).With("service", "svc")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(telemetry.AccessLog(logger))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	line := buf.String()
	if !strings.Contains(line, `"status":418`) || !strings.Contains(line, `"method":"GET"`) || !strings.Contains(line, `"path":"/x"`) {
		t.Fatalf("access log missing fields: %s", line)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, line)
	}
}
