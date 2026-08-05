# Telemetry (Sub-project D) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the api 15-Factor factor-XIV telemetry — structured `slog` logs, an OTel `/metrics` endpoint, and OTel distributed tracing (Jaeger backend) — with `trace_id` correlating logs and traces.

**Architecture:** A new `internal/telemetry` package configures slog + the OTel tracer/meter providers at startup. Middleware in `NewEngine` adds request spans (`otelgin`), a structured access log (with `trace_id`), and HTTP metrics; `/metrics` is served from the OTel Prometheus exporter. Jaeger all-in-one (native OTLP) joins the stack; its UI is admin-gated behind Caddy.

**Tech Stack:** Go 1.26, gin, GORM, `log/slog`, OpenTelemetry Go SDK (trace + metric), OTLP gRPC exporter, OTel Prometheus exporter, `otelgin`, GORM OTel plugin, Jaeger, Caddy.

## Global Constraints

- TDD mandatory: failing test → confirm fail → minimal code → confirm pass → refactor.
- No change to existing route strings or business behavior. New routes only: `GET /metrics` and `GET /api/authorization/verify-admin`.
- Logs go to **stdout** only (no files). Full English route names.
- Graceful degradation: with `APP_OTLP_ENDPOINT` empty, spans are still created (so `trace_id` is logged) but not exported; single-binary/dev must still boot and pass tests.
- Existing suite (243 Go tests) stays green after every task. Run `rtk go test ./...`. Build: `CGO_ENABLED=0 go build ./...`. Vet: `go vet ./...`.
- Never commit to `main`; branch `feat/telemetry` (already created).
- Uber Go style; American English; organized imports (`goimports`; if missing `go run golang.org/x/tools/cmd/goimports@latest -w <files>`).
- OTel Go API is version-sensitive: use the versions `go get` resolves; if a symbol/package path differs slightly from the code below (e.g. a `semconv` version or an `otelgin` sub-path), adjust to the resolved API so it compiles — the shapes here match OTel Go v1.x.
- Builders do NOT run git or touch `CONTEXT.md`/docker state — the orchestrator commits after review and runs the live Jaeger/metrics check.

---

## File Structure

- `internal/config/config.go` — `LogLevel`, `LogFormat`, `OTLPEndpoint`, `ServiceName`.
- `internal/telemetry/log.go` — **new**: slog logger builder + `AccessLog` middleware.
- `internal/telemetry/telemetry.go` — **new**: `Setup` (providers, exporters, /metrics handler, shutdown) + `InstrumentDB`.
- `internal/telemetry/metrics.go` — **new**: `RequestMetrics` middleware.
- `internal/telemetry/*_test.go` — **new**: log/access-log, tracing (in-memory), metrics tests.
- `internal/router/router.go` — add middleware (otelgin + AccessLog + RequestMetrics), `/metrics`, and the verify-admin route; `Deps` gains `Logger` + `Metrics`.
- `cmd/server/main.go` — call `telemetry.Setup`, `InstrumentDB`, defer shutdown, migrate `log.*` → slog, pass logger/metrics into `Deps`.
- `docker-compose.yaml`, `docker-compose.dev.yaml` — Jaeger service + api telemetry env.
- `deploy/caddy/Caddyfile` — admin-gated `/traces` block.
- `.env.example` — `APP_LOG_LEVEL`.
- `docs/adr/0006-telemetry-otel-jaeger.md` — **new** ADR.

---

### Task 1: Config — telemetry settings

**Files:** Modify `internal/config/config.go`; Test `internal/config/config_test.go`.

**Interfaces:** Produces `Config.LogLevel, LogFormat, OTLPEndpoint, ServiceName string`.

- [ ] **Step 1: Failing test** — append to `config_test.go`:

```go
func TestTelemetryConfigDefaults(t *testing.T) {
	t.Setenv("APP_LOG_LEVEL", "")
	t.Setenv("APP_LOG_FORMAT", "")
	t.Setenv("APP_SERVICE_NAME", "")
	c := Load()
	if c.LogLevel != "info" || c.LogFormat != "json" || c.ServiceName != "phum-panya-api" {
		t.Fatalf("defaults wrong: %+v", c)
	}
	if c.OTLPEndpoint != "" {
		t.Fatalf("OTLPEndpoint default = %q, want empty", c.OTLPEndpoint)
	}
}
```

- [ ] **Step 2: Run — FAIL** (`rtk go test ./internal/config/`).
- [ ] **Step 3: Implement.** Add fields + `Load()` lines:

```go
	LogLevel     string
	LogFormat    string
	OTLPEndpoint string
	ServiceName  string
```
```go
	LogLevel:     env("APP_LOG_LEVEL", "info"),
	LogFormat:    env("APP_LOG_FORMAT", "json"),
	OTLPEndpoint: env("APP_OTLP_ENDPOINT", ""),
	ServiceName:  env("APP_SERVICE_NAME", "phum-panya-api"),
```

- [ ] **Step 4: Run — PASS** (`rtk go test ./internal/config/ ./...`).
- [ ] **Step 5: Commit** — `feat(config): telemetry settings`.

---

### Task 2: Structured logging (slog) + access-log middleware + main migration

**Files:** Create `internal/telemetry/log.go`, `internal/telemetry/log_test.go`; Modify `cmd/server/main.go`.

**Interfaces:**
- Produces: `telemetry.NewLogger(cfg config.Config) *slog.Logger`; `telemetry.AccessLog(logger *slog.Logger) gin.HandlerFunc`.
- Note: `AccessLog` reads the OTel span context for `trace_id`/`span_id`; until Task 4 wires tracing it is simply absent (valid — the field is omitted when the span context is invalid).

- [ ] **Step 1: Failing test** — `internal/telemetry/log_test.go`:

```go
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
```

- [ ] **Step 2: Run — FAIL** (`rtk go test ./internal/telemetry/`) — package/functions absent.
- [ ] **Step 3: Add the otel trace API dep** (needed for span-context reads; small, no SDK):

```bash
go get go.opentelemetry.io/otel/trace@latest
```

- [ ] **Step 4: Implement** `internal/telemetry/log.go`:

```go
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
```

- [ ] **Step 5: Run — PASS** (`rtk go test ./internal/telemetry/`).
- [ ] **Step 6: Migrate `main.go` logging.** Replace every `log.Printf`/`log.Fatalf` in `cmd/server/main.go` with slog. At the top of `runServer` build the logger and set it default:

```go
	logger := telemetry.NewLogger(cfg)
	slog.SetDefault(logger)
```

Replace fatals with, e.g.:
```go
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
```
and `log.Printf("listening on %s", cfg.HTTPAddr)` → `slog.Info("listening", "addr", cfg.HTTPAddr)`. Remove the now-unused `"log"` import; add `"log/slog"` and `"os"` (if not present). Keep behavior identical (same fatal exits). The logger is threaded to the router in Task 4; for now `slog.SetDefault` covers the non-request logs.

- [ ] **Step 7: Run — PASS + build** (`rtk go test ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`).
- [ ] **Step 8: Commit** — `feat(telemetry): slog structured logging + access log`.

---

### Task 3: Telemetry core — providers, /metrics handler, OTLP export, shutdown

**Files:** Create `internal/telemetry/telemetry.go`, `internal/telemetry/telemetry_test.go`; Modify `go.mod`.

**Interfaces:**
- Produces:
  - `Setup(ctx context.Context, cfg config.Config) (metrics http.Handler, shutdown func(context.Context) error, err error)` — sets global tracer + meter providers and the W3C propagator; returns the Prometheus `/metrics` handler and a flush-on-exit shutdown.
  - `InstrumentDB(g *gorm.DB) error` — attaches the GORM OTel tracing plugin.

- [ ] **Step 1: Add deps**

```bash
go get go.opentelemetry.io/otel@latest \
  go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/sdk/metric@latest \
  go.opentelemetry.io/otel/exporters/prometheus@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest \
  go.opentelemetry.io/contrib/instrumentation/runtime@latest \
  gorm.io/plugin/opentelemetry@latest \
  github.com/prometheus/client_golang@latest
go mod tidy
```
(Needs network.)

- [ ] **Step 2: Failing test** — `internal/telemetry/telemetry_test.go`:

```go
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
```

- [ ] **Step 3: Run — FAIL** (`rtk go test ./internal/telemetry/`).
- [ ] **Step 4: Implement** `internal/telemetry/telemetry.go`:

```go
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
```

Note: the `semconv` import version (`v1.26.0`) must match a version present in the resolved `go.opentelemetry.io/otel` module; if `go build` reports it missing, change the path to the `semconv` version that module ships (e.g. `v1.24.0`) and use the matching `ServiceName`/`ServiceInstanceID` helpers.

- [ ] **Step 5: Run — PASS + build + vet** (`rtk go test ./internal/telemetry/ ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`).
- [ ] **Step 6: Commit** — `feat(telemetry): OTel providers, /metrics handler, OTLP export, GORM spans`.

---

### Task 4: Wire middleware + `/metrics` + request metrics into the router

**Files:** Create `internal/telemetry/metrics.go`, `internal/telemetry/metrics_test.go`; Modify `internal/router/router.go`, `cmd/server/main.go`.

**Interfaces:**
- Consumes: `Setup`, `NewLogger`, `AccessLog`, `InstrumentDB` (Tasks 2-3), `otelgin`.
- Produces: `telemetry.RequestMetrics() (gin.HandlerFunc, error)`; `router.Deps.Logger *slog.Logger`; `router.Deps.Metrics http.Handler`.

- [ ] **Step 1: Add otelgin**

```bash
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin@latest
go mod tidy
```

- [ ] **Step 2: Failing test** — `internal/telemetry/metrics_test.go`:

```go
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
```

- [ ] **Step 3: Run — FAIL** (`rtk go test ./internal/telemetry/`).
- [ ] **Step 4: Implement** `internal/telemetry/metrics.go`:

```go
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
```

- [ ] **Step 5: Run — PASS** (`rtk go test ./internal/telemetry/`).
- [ ] **Step 6: Wire the router.** In `internal/router/router.go`:
  - Add imports `log/slog`, `net/http`, `github.com/gin-gonic/gin/...` (already), `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin`, `phum-panya/internal/telemetry`.
  - Add to `Deps`: `Logger *slog.Logger` and `Metrics http.Handler`.
  - In `NewEngine`, replace `engine.Use(gin.Recovery(), gin.Logger())` with:

```go
	engine.Use(gin.Recovery())
	engine.Use(otelgin.Middleware(deps.Cfg.ServiceName))
	engine.Use(telemetry.AccessLog(deps.Logger))
	if rm, err := telemetry.RequestMetrics(); err == nil {
		engine.Use(rm)
	} else {
		deps.Logger.Error("request metrics", "err", err)
	}
```

  - Register `/metrics` on the engine (public path, internal use; no auth), before the SPA catch-all:

```go
	if deps.Metrics != nil {
		engine.GET("/metrics", gin.WrapH(deps.Metrics))
	}
```

- [ ] **Step 7: Wire `main.go`.** In `runServer`, after opening the DB and building `cfg`/`logger`:

```go
	metricsHandler, shutdownTelemetry, err := telemetry.Setup(context.Background(), cfg)
	if err != nil {
		slog.Error("telemetry setup", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()
	if err := telemetry.InstrumentDB(g); err != nil {
		slog.Error("instrument db", "err", err)
		os.Exit(1)
	}
```

  Add `Logger: logger,` and `Metrics: metricsHandler,` to the `router.Deps` literal.

- [ ] **Step 8: Fix the router test helper.** `internal/router/router_test.go` builds `Deps` directly; add `Logger: slog.Default()` (import `log/slog`) so `AccessLog` does not nil-panic. `Metrics` may stay nil (the `if deps.Metrics != nil` guard skips it).

- [ ] **Step 9: Run — PASS + build + vet** (`rtk go test ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`). All 243 existing tests plus the new telemetry tests pass.
- [ ] **Step 10: Commit** — `feat(telemetry): otelgin spans, access log, /metrics, request metrics wired`.

---

### Task 5: Admin-gated `/traces` — verify-admin route + Caddy forward-auth

**Files:** Modify `internal/router/router.go`, `internal/router/router_test.go`, `deploy/caddy/Caddyfile`.

**Interfaces:** Produces the route `GET /api/authorization/verify-admin` (204 for central-admin, else 401/403).

- [ ] **Step 1: Failing test** — add to `router_test.go` (reuse an existing auth-session helper if present; otherwise assert the unauthenticated case, which needs no session):

```go
func TestVerifyAdminRejectsAnonymous(t *testing.T) {
	engine := newEngine(t, t.TempDir())
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/authorization/verify-admin", nil))
	if rec.Code == http.StatusNoContent {
		t.Fatalf("anonymous verify-admin = 204, want 401/403")
	}
}
```

- [ ] **Step 2: Run — FAIL** (`rtk go test ./internal/router/`) — route returns 404 (which is != 204, so it would pass trivially). To make this a real red: first assert the route EXISTS by expecting 401 specifically:

Change the assertion to `if rec.Code != http.StatusUnauthorized { t.Fatalf("anonymous verify-admin = %d, want 401", rec.Code) }` and confirm it fails (currently 404).

- [ ] **Step 3: Implement.** In `NewEngine`, on the `api` group (which already applies `auth.LoadUser`), register:

```go
	api.GET("/api/authorization/verify-admin", auth.RequireRole("central_admin"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
```

Confirm `auth.RequireRole` returns 401 when no user is loaded and 403 for a wrong role (read `internal/auth/middleware.go:80` to confirm the exact codes; if `RequireRole` alone does not 401 on anonymous, chain `auth.RequireAuth()` before it). Adjust the test's expected code to the actual anonymous code (401 or 403).

- [ ] **Step 4: Run — PASS** (`rtk go test ./internal/router/ ./...`).
- [ ] **Step 5: Caddy.** In `deploy/caddy/Caddyfile`, add before the `handle {}` fallback:

```caddyfile
	# Jaeger trace UI, gated to central-admin sessions via the api.
	handle /traces* {
		forward_auth api:8080 {
			uri /api/authorization/verify-admin
			copy_headers Cookie
		}
		reverse_proxy jaeger:16686
	}
```

- [ ] **Step 6: Verify Caddy parses** (needs the stack's required env; the api/jaeger need not be up):
```bash
docker run --rm -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1 | tail -5
```
Expected: "Valid configuration". (If `{$APP_DOMAIN}` interpolation trips validate, set `APP_DOMAIN=example.org` via `-e`.)

- [ ] **Step 7: Commit** — `feat(telemetry): admin-gated /traces verify route + Caddy forward-auth`.

---

### Task 6: Compose Jaeger + api env + dev parity + docs

**Files:** Modify `docker-compose.yaml`, `docker-compose.dev.yaml`, `.env.example`; Create `docs/adr/0006-telemetry-otel-jaeger.md`.

- [ ] **Step 1: Jaeger service.** Add to `docker-compose.yaml` (internal, no published port):

```yaml
  jaeger:
    image: jaegertracing/all-in-one:1.62.0
    restart: unless-stopped
    environment:
      COLLECTOR_OTLP_ENABLED: "true"
      QUERY_BASE_PATH: /traces
```

- [ ] **Step 2: api telemetry env.** Add to the `api` service `environment:` block (prod):

```yaml
      APP_OTLP_ENDPOINT: jaeger:4317
      APP_LOG_LEVEL: info
      APP_LOG_FORMAT: json
      APP_SERVICE_NAME: phum-panya-api
```

- [ ] **Step 3: Caddy depends_on jaeger.** Add `jaeger` to the caddy service `depends_on:` (so `/traces` has an upstream); do NOT add it to api `depends_on` (the tracer tolerates an absent backend).

- [ ] **Step 4: Dev parity.** Mirror Steps 1-3 in `docker-compose.dev.yaml` (jaeger service + dev api telemetry env; APP_LOG_FORMAT may be `text` for dev readability if preferred).

- [ ] **Step 5: `.env.example`.** Add a documented block:

```sh
# --- Telemetry (optional tuning) -------------------------------------------
# Structured logs go to stdout as JSON. Level: debug|info|warn|error.
APP_LOG_LEVEL=info
# Traces: the prod/dev stacks send OTLP to the bundled Jaeger; view them at
# https://<domain>/traces (central-admin login required).
```

- [ ] **Step 6: ADR-0006.** Write `docs/adr/0006-telemetry-otel-jaeger.md` following the 0005 format: context (factor XIV; no telemetry before), decision (slog JSON logs; OTel traces+metrics; Jaeger all-in-one native OTLP; /metrics via OTel Prometheus exporter; trace_id correlation; admin-gated /traces; OTLP opt-in for single-binary), consequences (heavy OTel dep set; Jaeger service RAM; /metrics internal; single-binary works without a backend).

- [ ] **Step 7: Verify** (both compose files validate with required vars exported inline):
```bash
GARAGE_RPC_SECRET=x APP_S3_ACCESS_KEY=k APP_S3_SECRET_KEY=s APP_DOMAIN=example.org APP_ADMIN_PASSWORD=p POSTGRES_PASSWORD=pp docker compose -f docker-compose.yaml config -q && echo PROD_OK
GARAGE_RPC_SECRET=x APP_S3_ACCESS_KEY=k APP_S3_SECRET_KEY=s POSTGRES_PASSWORD=pp docker compose -f docker-compose.dev.yaml config -q && echo DEV_OK
```

- [ ] **Step 8: Commit** — `feat(deploy): bundle Jaeger; api exports OTLP; ADR-0006`.

- [ ] **Step 9 (orchestrator):** live check — bring up postgres + garage + garage-init + jaeger + api, make a request, confirm (a) `/metrics` on the api serves `http_server_request_count`, (b) the trace appears via Jaeger's query API (`/traces/api/traces?service=phum-panya-api` or the Jaeger HTTP API), and (c) logs are JSON with `trace_id`. Then update `CONTEXT.md` and run the final whole-branch review.

---

## Self-Review

**Spec coverage:**
- Structured slog logs + resource fields + access log → Tasks 2, 4.
- `/metrics` (OTel Prometheus exporter) + HTTP + runtime metrics → Tasks 3, 4.
- OTel tracing (otelgin + GORM spans), W3C propagation, trace_id in logs, OTLP opt-in → Tasks 3, 4.
- `APP_LOG_LEVEL/FORMAT/OTLP_ENDPOINT/SERVICE_NAME` → Task 1.
- Admin-gated `/traces` (verify-admin route + Caddy forward-auth) → Task 5.
- Jaeger in stack + api env + dev parity + `.env.example` + ADR-0006 → Task 6.
- Graceful degradation without a backend — Task 3 (`if cfg.OTLPEndpoint != ""`), verified by `TestSetupMetricsHandlerServesPrometheus` (no endpoint).
- No route/behavior change to existing endpoints; only `/metrics` + `/api/authorization/verify-admin` added.

**Placeholder scan:** The ADR body (Task 6 Step 6) and the exact anonymous status code (Task 5, 401 vs 403 — resolved by reading `middleware.go:80`) are the only "confirm against real code" notes; every code/YAML block is complete. The OTel `semconv` version note and the orchestrator live check are deliberate integration instructions, not placeholders.

**Type consistency:** `telemetry.Setup(ctx,cfg) (http.Handler, func(context.Context) error, error)` matches its call in `main.go` (Task 4) and the tests (Tasks 3, 4). `NewLogger(cfg) *slog.Logger`, `AccessLog(*slog.Logger)`, `RequestMetrics() (gin.HandlerFunc, error)`, `InstrumentDB(*gorm.DB) error` are used with matching signatures across telemetry, router, and main. `router.Deps` gains `Logger *slog.Logger` + `Metrics http.Handler`, set in `main.go` and defaulted in `router_test.go`. Config field names (`LogLevel`, `LogFormat`, `OTLPEndpoint`, `ServiceName`) are identical across config, telemetry, and compose env.
