# Sub-project D — Telemetry: logs + metrics + tracing (design)

Date: 2026-08-05
Branch: `feat/telemetry`
Status: approved design, ready for implementation plan

## Program context

Sub-project **D** of the 15-Factor + Hexagonal compliance program (see
`docs/superpowers/specs/2026-08-05-hexagonal-core-refactor-design.md`).
A (hexagonal core), B (media → Garage), and C (shared throttle) are merged.
D delivers 15-Factor **factor XIV (Telemetry)**: structured logs, metrics, and
distributed tracing, with trace-based correlation across replicas.

Order: A ✓ → B ✓ → C ✓ → **D** → E (migrations-as-release + multi-replica).

## Locked decisions (from brainstorming)

- **Full depth**: structured logs + a `/metrics` endpoint + OpenTelemetry tracing.
- **One spec/plan** covering all three signals, in one branch.
- **Ship a tracing backend** in the compose stack: **Jaeger all-in-one** with its
  native OTLP receiver (no separate collector — the lean version).
- **Jaeger UI behind Caddy at `/traces`, admin-gated** via Caddy `forward_auth`
  to a new api verify endpoint.
- Logging via **stdlib `log/slog`** (zero-dep), JSON to stdout.
- Metrics via the **OTel SDK Prometheus exporter** — one instrumentation SDK
  (OTel) for both metrics and traces.
- OTLP trace export is configured only when `APP_OTLP_ENDPOINT` is set;
  `trace_id` still appears in logs without a backend (single-binary/dev-simple).

## 1. Goal and non-goals

**Goal:** Give the api the three telemetry signals with trace-based correlation
and a viewable tracing backend, without changing business logic or routes.

**Non-goals:**
- No change to business logic or existing route strings (telemetry is
  cross-cutting middleware + startup wiring, plus one new internal auth route
  and `/metrics`).
- No log files — stdout only (the platform owns log routing).
- SQLite/single-binary keep working: telemetry degrades gracefully with no
  OTLP backend (spans created, not exported).

## 2. Structured logging (foundation)

Replace `gin.Logger()` and the stdlib `log.*` calls with **`log/slog`**:

- A JSON handler to **stdout**, configured once at startup and set as the slog
  default. Level from `APP_LOG_LEVEL` (`debug|info|warn|error`, default `info`);
  `APP_LOG_FORMAT` (`json` default | `text` for local dev).
- `cmd/server/main.go`'s `log.Printf`/`log.Fatalf` become `slog` calls
  (`slog.Error(...)` then `os.Exit(1)` where a fatal is required).
- A new **access-log middleware** (`internal/telemetry` or `internal/httpx`)
  logs one structured line per request: `method`, `path`, `status`,
  `latency_ms`, `bytes`, `client_ip`, and `trace_id`/`span_id` (§4).
- `gin.Recovery()` stays; recovered panics are logged via slog at `error`.
- Every line carries resource fields `service` and `instance` (hostname) so
  multi-replica logs are attributable.

## 3. Metrics

Use the **OpenTelemetry metrics SDK** with its **Prometheus exporter**, serving
`GET /metrics` (Prometheus text format) on the api.

- HTTP metrics: `http_server_request_count` and an
  `http_server_request_duration` histogram (labels: route template, method,
  status), recorded by the request middleware.
- Go runtime metrics via
  `go.opentelemetry.io/contrib/instrumentation/runtime` (`go_*` / `process_*`).
- `/metrics` is **internal only**: Caddy never routes it publicly. A scraper
  reaches `api:8080/metrics` on the compose network directly (the api has no
  published port). The single OTel `MeterProvider` backs both `/metrics` and any
  future OTLP metric push.

## 4. Distributed tracing

Adopt the **OpenTelemetry trace SDK** with **`otelgin`** middleware:

- One span per HTTP request, honoring the inbound W3C `traceparent` header (a
  request keeps one trace across replicas/services) and creating a root span
  otherwise. W3C `TraceContext` propagator is set globally.
- The active span's `trace_id`/`span_id` are injected into every slog line —
  this is the correlation mechanism (no separate request-ID). The response
  carries the trace id in a header for debugging.
- The DB is instrumented with the GORM OpenTelemetry plugin
  (`gorm.io/plugin/opentelemetry/tracing`) so queries appear as child spans.
- A `TracerProvider` is always created (so `trace_id` exists in logs even with
  no backend); the **OTLP gRPC exporter is added only when `APP_OTLP_ENDPOINT`
  is set**.

## 5. Startup wiring (composition)

New package **`internal/telemetry`**:

```go
// Setup builds the slog logger, the OTel tracer + meter providers, the
// Prometheus /metrics handler, and (when cfg.OTLPEndpoint != "") the OTLP trace
// exporter. It returns the /metrics http.Handler and a shutdown that flushes
// spans on exit.
func Setup(ctx context.Context, cfg config.Config) (metrics http.Handler, shutdown func(context.Context) error, err error)

// InstrumentDB attaches the GORM OTel tracing plugin to g.
func InstrumentDB(g *gorm.DB) error
```

- Resource attributes: `service.name` (`APP_SERVICE_NAME`, default
  `phum-panya-api`), `service.instance.id` (hostname).
- `cmd/server/main.go` calls `Setup` early, `defer`s `shutdown`, calls
  `InstrumentDB(g)`, and passes the metrics handler into `router.Deps`.
- `internal/router.NewEngine` adds `otelgin.Middleware(serviceName)` and the
  access-log middleware (replacing `gin.Logger()`), and registers
  `GET /metrics` (the handler from `Deps`).
- New config fields: `LogLevel`, `LogFormat`, `OTLPEndpoint`, `ServiceName`.

## 6. Admin-gated Jaeger UI (Caddy forward-auth)

- New internal api route **`GET /api/authorization/verify-admin`** (full English
  name): returns `204` when the forwarded session cookie belongs to a
  `central_admin`, else `401`/`403`. It reuses `auth.LoadUser` +
  `auth.RequireRole("central_admin")` (registered on the `api` group).
- `deploy/caddy/Caddyfile`: a `handle /traces*` block runs
  `forward_auth api:8080 { uri /api/authorization/verify-admin ... }`, then
  `reverse_proxy jaeger:16686` on success. Jaeger runs with
  `QUERY_BASE_PATH=/traces` so its UI assets resolve under that path.

## 7. Deploy (compose)

- Add **`jaeger`** (`jaegertracing/all-in-one`) to `docker-compose.yaml` + dev:
  `COLLECTOR_OTLP_ENABLED=true`, `QUERY_BASE_PATH=/traces`, internal only
  (**no published port**), volumes for its badger store (optional; memory is
  fine at this scale).
- api env (prod + dev): `APP_OTLP_ENDPOINT=jaeger:4317`, `APP_LOG_LEVEL=info`,
  `APP_LOG_FORMAT=json`, `APP_SERVICE_NAME=phum-panya-api`. api `depends_on`
  jaeger healthy is NOT required (OTLP export tolerates an absent backend and
  retries; do not block boot on the tracer).
- `.env.example`: document `APP_LOG_LEVEL`.
- **ADR-0006**: records the telemetry decision (slog + OTel; Jaeger backend;
  `/metrics`; admin-gated UI; OTLP opt-in for single-binary).

## 8. Testing (TDD)

- **slog**: build the handler over a `bytes.Buffer`, assert a log call emits
  JSON with the expected keys and level.
- **access-log + trace middleware**: over a test engine, assert the response
  carries a non-empty trace-id header and that an inbound `traceparent` is
  echoed as the same trace id; assert the access-log line contains `trace_id`.
- **tracing**: use OTel's in-memory `tracetest.SpanRecorder`/exporter to assert
  one span per request with the expected name and `http.*` attributes — no live
  backend needed.
- **/metrics**: after a request through the test engine, `GET /metrics` returns
  `200` and Prometheus text containing `http_server_request_count` (a substring
  match — the Prometheus exporter appends `_total` to counters and maps dots to
  underscores, so assert a substring, not an exact metric name). The HTTP
  instruments are custom, explicitly named counters/histograms in the request
  middleware (not exporter-auto names), so the names are deterministic.
- **verify-admin route**: admin session → `204`; non-admin/no session →
  `401`/`403` (reuses the existing auth test helpers).
- The existing 243 tests stay green (routes/behavior unchanged; log output is
  not asserted by existing tests).
- **Live check (orchestrator):** bring up api + Jaeger, make a request, confirm
  the trace appears in Jaeger's API and `/metrics` serves the counters.

## 9. New dependencies

`go.opentelemetry.io/otel`, `.../sdk`, `.../sdk/metric`,
`.../exporters/otlp/otlptrace/otlptracegrpc`, `.../exporters/prometheus`,
`.../contrib/instrumentation/github.com/gin-gonic/gin/otelgin`,
`.../contrib/instrumentation/runtime`,
`gorm.io/plugin/opentelemetry/tracing`, and `github.com/prometheus/client_golang`
(transitive, for the `/metrics` handler). Substantial, and accepted as the
cost of full factor-XIV telemetry.

## Success criteria

- App logs are JSON on stdout with `service`/`instance` and, within a request,
  `trace_id`/`span_id`; level honors `APP_LOG_LEVEL`.
- `GET /metrics` serves Prometheus metrics including HTTP request count/duration
  and Go runtime metrics; it is not routed publicly by Caddy.
- Each request creates an OTel span; with `APP_OTLP_ENDPOINT=jaeger:4317` the
  trace is visible in Jaeger; `trace_id` correlates logs to traces.
- `/traces` is reachable only for a central-admin session (Caddy forward-auth).
- No route/behavior change to existing endpoints; single-binary works with no
  backend (spans unexported, `trace_id` still logged). Full suite green.
