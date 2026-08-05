package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"phum-panya/internal/config"
)

// Setup configures the global OTel tracer + meter providers and the W3C
// propagator, returns the Prometheus /metrics handler, and a shutdown that
// flushes both providers. The OTLP trace exporter is added only when
// cfg.OTLPEndpoint is set; otherwise spans are created (so trace_id is logged)
// but not exported.
func Setup(ctx context.Context, cfg config.Config) (http.Handler, func(context.Context) error, error) {
	host, _ := os.Hostname()
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceInstanceID(host),
	))
	if err != nil {
		return nil, nil, err
	}

	reg := promclient.NewRegistry()
	promExp, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, nil, err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(promExp))
	otel.SetMeterProvider(mp)
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return nil, nil, err
	}

	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if cfg.OTLPEndpoint != "" {
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, nil, err
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	shutdown := func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), shutdown, nil
}

// InstrumentDB attaches the GORM OpenTelemetry tracing plugin so queries become
// child spans of the request. Metrics from the plugin are disabled (HTTP
// metrics are recorded separately).
func InstrumentDB(g *gorm.DB) error {
	return g.Use(tracing.NewPlugin(tracing.WithoutMetrics()))
}
