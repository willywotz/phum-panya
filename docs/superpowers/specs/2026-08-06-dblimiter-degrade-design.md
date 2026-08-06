# DBLimiter degrade-to-in-process on store failure

Date: 2026-08-06
Status: Approved (design)
Scope: `internal/auth/dblimiter.go`, `cmd/server/main.go`, tests

## Problem

`DBLimiter` is the Postgres-backed login throttle shared across api replicas. Its
methods swallow database errors. When Postgres is unreachable:

- `Allowed` runs `Count`; on error `n` stays `0`, so `n < max` returns `true` — the
  login is allowed.
- `Fail` runs `Create`; on error nothing is recorded.
- No log line and no metric are produced.

The result is a **silent fail-open**: during a Postgres outage every login is allowed,
no failures are counted, and brute-force protection disappears with no signal. This is
a security gap, because the outage window is exactly when an attacker may be able to
stress the database.

## Decision

When a Postgres call inside `DBLimiter` fails, that call **degrades to a per-replica
in-process `Throttle`** (same `max` and `window`) instead of failing open. Each failed
call also logs a WARN and increments a counter, so the degraded window is visible.

Detection is **per-call**: each `Allowed`/`Fail`/`Reset` checks its own result and
degrades only that call. There is no circuit breaker and no recovery state to track.

### Alternatives considered

- **Fail-open (keep, but log)** — availability first, but leaves an unthrottled window.
- **Fail-closed** — a database blip becomes a total login lockout (self-inflicted auth
  outage). Rejected.
- **Degrade to in-process** — chosen. Login keeps working and stays throttled per
  replica. An attacker can spread attempts across replicas during the outage, but each
  replica still throttles; this is the accepted best-effort trade-off.

## Design

### Interface

The `Limiter` interface (`Allowed(key) bool`, `Fail(key)`, `Reset(key)` — no error, no
context) is **unchanged**. Degradation is fully internal to `DBLimiter`.

### `DBLimiter` fields

Add three fields:

- `fallback *Throttle` — an in-process `Throttle` built with the same `max`/`window`.
- `logger *slog.Logger` — for WARN lines.
- `errs metric.Int64Counter` — the store-error counter.

### Methods

- `Allowed(key)`: run the `Count`. If `res.Error != nil` → `storeError("allowed", err)`
  and `return l.fallback.Allowed(key)`. Else `return n < max`.
- `Fail(key)`: run the prune-delete (its error is non-critical and ignored) and the
  `Create`. If the `Create` errors → `storeError("fail", err)` and `l.fallback.Fail(key)`.
- `Reset(key)`: run the `Delete`. If it errors → `storeError("reset", err)` and
  `l.fallback.Reset(key)`.
- `storeError(op string, err error)`:
  - `l.errs.Add(context.Background(), 1, metric.WithAttributes(attribute.String("op", op)))`
  - `l.logger.Warn("login throttle store error", "op", op, "err", err)`

`context.Background()` is used for the counter because the `Limiter` interface carries no
request context; the metric does not need trace correlation.

### Metric

Name: `login_throttle_store_error_count` (full English, matches the `_count` convention
used by `http_server_request_count`). Attribute: `op` = `allowed` | `fail` | `reset`.
Built once in `NewDBLimiter` from `otel.Meter("phum-panya/auth")`.

### Logging

Every failed store call logs one WARN (chosen over transition-only logging for
simplicity — no state flag). The counter carries per-attempt volume for dashboards.

### Constructor and wiring

`NewDBLimiter` gains a `logger *slog.Logger` parameter, builds its own counter, and
returns `(*DBLimiter, error)` (counter creation can fail, mirroring
`telemetry.RequestMetrics`). `newLimiter` in `cmd/server/main.go` threads the existing
`logger` value through and propagates the error (exit on failure, consistent with the
surrounding setup code). This is the only call-site change.

## Accepted trade-off

Failures recorded in the in-process fallback during an outage are **not** written back
to Postgres on recovery. After recovery, `Allowed` reads the clean DB count, so
outage-time failures do not persist. This is inherent to degrade-to-local and matches
the best-effort, per-replica decision.

## Testing (TDD)

Force store errors by pointing a `*gorm.DB` at a closed connection (or a driver that
returns errors), then assert:

1. `Allowed` falls back — returns `true` under threshold and `false` once the in-process
   fallback reaches `max` failures via `Fail`.
2. `Fail` records into the fallback (verified through a subsequent `Allowed`).
3. Store errors increment `login_throttle_store_error_count` and emit a WARN (captured
   with a `slog` test handler and an in-memory / manual metric reader).
4. Existing happy-path DB-backed tests continue to pass unchanged.

## Out of scope

- Circuit breaker / backoff.
- Carrying fallback state back into Postgres on recovery.
- Adding `context` or `error` to the `Limiter` interface.
