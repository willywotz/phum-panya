// Package telemetry wires structured logging, metrics, and tracing.
package telemetry

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"

	"phum-panya/internal/config"
)

// NewLogger builds the process logger: JSON (or text) to stdout at the
// configured level, tagged with the service name and this host's name so
// multi-replica logs are attributable.
func NewLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}
	var h slog.Handler
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	host, _ := os.Hostname()
	return slog.New(h).With("service", cfg.ServiceName, "instance", host)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// AccessLog logs one structured line per request, including the active span's
// trace_id/span_id (present once tracing is wired) so a log line ties to a
// trace. It also echoes the trace id in the X-Trace-Id response header.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		start := time.Now()
		sc := trace.SpanContextFromContext(c.Request.Context())
		if sc.IsValid() {
			c.Header("X-Trace-Id", sc.TraceID().String())
		}
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"client_ip", c.ClientIP(),
		}
		if sc.IsValid() {
			attrs = append(attrs, "trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
		}
		logger.Info("request", attrs...)
	}
}
