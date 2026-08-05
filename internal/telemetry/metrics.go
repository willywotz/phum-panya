package telemetry

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RequestMetrics records an HTTP request counter and a duration histogram,
// labelled by route template, method, and status, from the global meter
// (set by Setup). Call it after Setup.
func RequestMetrics() (gin.HandlerFunc, error) {
	meter := otel.Meter("phum-panya/http")
	count, err := meter.Int64Counter("http_server_request_count")
	if err != nil {
		return nil, err
	}
	dur, err := meter.Float64Histogram("http_server_request_duration", metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		attrs := metric.WithAttributes(
			attribute.String("http.route", route),
			attribute.String("http.method", c.Request.Method),
			attribute.Int("http.status_code", c.Writer.Status()),
		)
		count.Add(c.Request.Context(), 1, attrs)
		dur.Record(c.Request.Context(), float64(time.Since(start).Milliseconds()), attrs)
	}, nil
}
