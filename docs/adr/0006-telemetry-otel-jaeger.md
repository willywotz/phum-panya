# Telemetry: slog logs, OpenTelemetry traces/metrics, bundled Jaeger

Status: accepted

Context: the api had no telemetry — 15-Factor factor XIV (Telemetry) was
unmet. `cmd/server/main.go` logged with the standard `log` package (no
structure, no levels), there was no `/metrics` endpoint, and there was no way
to trace a request across `gin` → GORM → Postgres/Garage to diagnose latency
or errors in the container stack. Sub-project D
(`docs/superpowers/plans/2026-08-05-telemetry.md`) adds structured logging, an
OTel `/metrics` endpoint, and OTel distributed tracing (Jaeger backend), with
`trace_id` correlating logs and traces, while keeping the single-binary
deployment (ADR-0001) working without any telemetry backend.

## Decision

`internal/telemetry` (`log.go`, `telemetry.go`, `metrics.go`) is the one new
adapter package for this concern:

- **Logs.** `NewLogger` builds a `log/slog` logger to stdout only (12-Factor
  logs-as-event-stream), JSON by default (`APP_LOG_FORMAT=json`, `text`
  available for local readability), tagged with `service`/`instance`.
  `AccessLog` middleware emits one structured line per request (method, path,
  status, latency, bytes, client IP) and includes `trace_id`/`span_id` when a
  span is active, so a log line ties directly to a trace.
- **Traces.** `otelgin` creates a span per request; the GORM OTel plugin
  (`gorm.io/plugin/opentelemetry/tracing`) adds child spans for DB queries.
  `Setup` always builds a `TracerProvider` (so `trace_id` is always present in
  logs) but only attaches the OTLP gRPC exporter when `APP_OTLP_ENDPOINT` is
  set — this is what lets the single-binary deployment run without a Jaeger
  (or any) backend. When `APP_OTLP_ENDPOINT=jaeger:4317` (both compose
  stacks), Jaeger's all-in-one image receives spans over its native OTLP
  collector (`COLLECTOR_OTLP_ENABLED=true`); no separate collector service.
- **Metrics.** `Setup` registers an OTel `MeterProvider` backed by the OTel
  Prometheus exporter, serving `GET /metrics` in Prometheus text format
  (Go runtime metrics + `RequestMetrics` middleware's HTTP request
  count/duration, labelled by route/method/status).
- **Access control.** Jaeger's UI (`QUERY_BASE_PATH=/traces`) is not exposed
  directly; Caddy gates `/traces*` behind `forward_auth` to a new
  `GET /api/authorization/verify-admin` route (`central_admin` role only),
  then reverse-proxies to `jaeger:16686`. `/metrics` stays unauthenticated but
  is never published — both compose stacks keep `jaeger` and the api's own
  port off the host, reachable only inside the compose network.

## Why

Structured stdout logs, black-box `/metrics`, and a trace backend are the
three telemetry pillars 15-Factor factor XIV calls for, and OTel is the
vendor-neutral way to produce all three from one instrumentation surface
(traces, metrics) plus `slog` for logs — matching the existing preference
(ADR-0003, ADR-0004, ADR-0005) for reusing one well-supported backing service
per concern instead of bespoke logging/metrics code. Jaeger's all-in-one
image is chosen over a separate OTel Collector + Jaeger pair because it
accepts OTLP directly (`COLLECTOR_OTLP_ENABLED=true`), so one container
covers ingestion, storage, and the query UI — appropriate for the
single-VPS deploy model. Making `APP_OTLP_ENDPOINT` opt-in (empty by
default) keeps `Setup` from requiring a live exporter connection, so
`go run ./cmd/server` and the SQLite single-binary path (ADR-0001) still
start and log `trace_id`s with no Jaeger present.

## Considered options

- **Ship a separate OTel Collector in front of Jaeger.** Rejected: adds a
  second container and config file for no benefit here — Jaeger's
  all-in-one already speaks OTLP natively, and the stack has no second
  trace consumer to fan the Collector out to.
- **Use a hosted/SaaS tracing backend instead of bundling Jaeger.**
  Rejected: contradicts the self-hosted, single-VPS deploy model
  (ADR-0001, ADR-0003, ADR-0004); it would also make `APP_OTLP_ENDPOINT`
  effectively mandatory instead of an opt-in default.
- **Expose Jaeger's UI directly on a published port instead of gating it
  behind Caddy forward-auth.** Rejected: trace data can include request
  paths and identifiers; gating it behind the same `central_admin` session
  the rest of the admin surface uses avoids adding a second auth
  mechanism or a second public port.

## Consequences

- **Heavier dependency set.** The api now pulls the OpenTelemetry Go SDK
  (trace + metric), the OTLP gRPC exporter, the OTel Prometheus exporter,
  `otelgin`, and the GORM OTel plugin; `gin` is bumped to 1.12 to satisfy
  `otelgin`'s constraint. `go build` and image size grow accordingly.
- **Jaeger adds a stateful-ish container.** `jaegertracing/all-in-one` keeps
  trace data in memory by default (no persistent volume here), so restarting
  the `jaeger` container drops trace history; it adds to the host's RAM
  budget but needs no new volume or backup job.
- **`/metrics` is internal-only.** Both compose stacks keep `jaeger` and the
  api unpublished; `/metrics` is reachable only from inside the compose
  network (e.g. a future Prometheus scraper joining that network), never
  from the host or internet.
- **The single-binary path is unaffected functionally.** With
  `APP_OTLP_ENDPOINT` unset, the api still builds, starts, and serves
  requests — spans are created and `trace_id` is logged, but nothing is
  exported, and `/metrics` still works locally.
