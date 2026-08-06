# DBLimiter Degrade-to-In-Process Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `DBLimiter` degrade to a per-replica in-process `Throttle` (with a WARN log and a metric) when its Postgres store fails, instead of silently failing open.

**Architecture:** Degradation is internal to `DBLimiter`; the `Limiter` interface is unchanged. Each `Allowed`/`Fail`/`Reset` checks its own gorm result and, on error, logs + increments a counter + delegates to an embedded fallback `Throttle` built with the same `max`/`window`.

**Tech Stack:** Go, gorm, OpenTelemetry metrics (`go.opentelemetry.io/otel`), `log/slog`.

## Global Constraints

- ASD-STE100 Simplified Technical English in prose/docs.
- 15-Factor App and Hexagonal architecture compliance — `internal/auth` domain code uses port interfaces, imports no HTTP/transport infra.
- API route names full English (no new routes here, but honor if touched).
- TDD mandatory: failing test → confirm fail → minimal code → confirm pass.
- Uber Go style; American English names; organized (path-sorted) imports; minimal comments.
- Metric name full English with `_count` suffix: `login_throttle_store_error_count`.
- Commit prefix commands with `rtk`.
- Spec: `docs/superpowers/specs/2026-08-06-dblimiter-degrade-design.md`.

---

## File Structure

- Modify: `internal/auth/dblimiter.go` — add `fallback`/`logger`/`errs` fields, widen `NewDBLimiter`, add `storeError`, add degrade branches.
- Modify: `internal/auth/dblimiter_test.go` — update helper to new signature; add degrade tests.
- Modify: `cmd/server/main.go:196-201` — `newLimiter` threads `logger` and returns an error; caller handles it.

Reference — current signature (`internal/auth/dblimiter.go:25`):
`func NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration) *DBLimiter`

Reference — in-process fallback type (`internal/auth/throttle.go:30`):
`func NewThrottle(clk clock.Clock, max int, window time.Duration) *Throttle`

---

### Task 1: Widen `NewDBLimiter` (logger + error + fallback + counter), keep behavior unchanged

Pure refactor: add the new fields and constructor shape, wire all call sites, but do
**not** add degrade branches yet. Existing behavior and tests must still pass. The OTel
global meter defaults to a no-op provider when `telemetry.Setup` has not run, so building
the counter returns a nil error in tests.

**Files:**
- Modify: `internal/auth/dblimiter.go`
- Modify: `internal/auth/dblimiter_test.go`
- Modify: `cmd/server/main.go:196-201` and the call site at `cmd/server/main.go:167`

**Interfaces:**
- Consumes: `auth.NewThrottle(clk, max, window) *Throttle` (existing); `otel.Meter(name).Int64Counter(name) (metric.Int64Counter, error)`.
- Produces: `auth.NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration, logger *slog.Logger) (*DBLimiter, error)`. New unexported fields on `DBLimiter`: `fallback *Throttle`, `logger *slog.Logger`, `errs metric.Int64Counter`.

- [ ] **Step 1: Update the test helper and existing call sites to the new signature**

In `internal/auth/dblimiter_test.go`, add a helper and route the three existing tests through it. Replace the three `auth.NewDBLimiter(g, fake, 3, time.Minute)` calls with `newTestLimiter(t, g, fake, 3, time.Minute)`.

```go
func newTestLimiter(t *testing.T, g *gorm.DB, clk clock.Clock, max int, window time.Duration) *auth.DBLimiter {
	t.Helper()
	l, err := auth.NewDBLimiter(g, clk, max, window, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	return l
}
```

Add imports `io`, `log/slog` to the test file. Update the three call sites:
`l := newTestLimiter(t, newLimiterDB(t), fake, 3, time.Minute)` (and the `a`/`b` pair in `TestDBLimiterSharedAcrossInstances` using the shared `g`).

- [ ] **Step 2: Run the tests to confirm they FAIL to compile**

Run: `rtk go test ./internal/auth/ -run TestDBLimiter`
Expected: FAIL — build error, `NewDBLimiter` wants 4 args / single return.

- [ ] **Step 3: Widen `NewDBLimiter` and add fields (no degrade behavior yet)**

In `internal/auth/dblimiter.go`, update the struct and constructor. Add imports: `log/slog`, `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/metric`. Method bodies for `Allowed`/`Fail`/`Reset` are unchanged in this task.

```go
type DBLimiter struct {
	db       *gorm.DB
	clk      clock.Clock
	max      int
	window   time.Duration
	fallback *Throttle
	logger   *slog.Logger
	errs     metric.Int64Counter
}

// NewDBLimiter returns a DB-backed Limiter that degrades to a per-replica
// in-process Throttle when the database is unreachable.
func NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration, logger *slog.Logger) (*DBLimiter, error) {
	errs, err := otel.Meter("phum-panya/auth").Int64Counter("login_throttle_store_error_count")
	if err != nil {
		return nil, err
	}
	return &DBLimiter{
		db:       g,
		clk:      clk,
		max:      max,
		window:   window,
		fallback: NewThrottle(clk, max, window),
		logger:   logger,
		errs:     errs,
	}, nil
}
```

- [ ] **Step 4: Thread `logger` and the error through `cmd/server/main.go`**

Change `newLimiter` (`cmd/server/main.go:196`) to accept `logger` and return an error:

```go
func newLimiter(g *gorm.DB, cfg config.Config, clk clock.Clock, logger *slog.Logger) (auth.Limiter, error) {
	if cfg.UsesDBThrottle() {
		return auth.NewDBLimiter(g, clk, loginThrottleMax, loginThrottleWindow, logger)
	}
	return auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow), nil
}
```

Update the call site (`cmd/server/main.go:167`). Replace the inline `Throttle: newLimiter(g, cfg, clk),` field by building the limiter before the `router.Deps` literal:

```go
	limiter, err := newLimiter(g, cfg, clk, logger)
	if err != nil {
		slog.Error("build limiter", "err", err)
		os.Exit(1)
	}
```

and set `Throttle: limiter,` in the `router.Deps` literal.

- [ ] **Step 5: Run the auth tests and a build to confirm PASS**

Run: `rtk go test ./internal/auth/... && rtk go build ./...`
Expected: PASS; build succeeds.

- [ ] **Step 6: Commit**

```bash
rtk git add internal/auth/dblimiter.go internal/auth/dblimiter_test.go cmd/server/main.go
rtk git commit -m "refactor(auth): widen NewDBLimiter with logger, error, fallback

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Degrade `Allowed`/`Fail`/`Reset` to the fallback on store error

Add the `storeError` helper and the per-call error branches, driven by tests that force a
closed database connection.

**Files:**
- Modify: `internal/auth/dblimiter.go`
- Modify: `internal/auth/dblimiter_test.go`

**Interfaces:**
- Consumes: `DBLimiter.fallback *Throttle`, `DBLimiter.logger *slog.Logger`, `DBLimiter.errs metric.Int64Counter` (from Task 1).
- Produces: unexported `func (l *DBLimiter) storeError(op string, err error)`. Public method behavior: on store error, `Allowed`/`Fail`/`Reset` delegate to `l.fallback`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/auth/dblimiter_test.go`. Add imports `bytes`, `strings`, `context`, and the OTel test deps `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/sdk/metric`. Helper to force a store error by closing the underlying connection:

```go
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	g := newLimiterDB(t)
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatalf("g.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return g
}

func TestDBLimiterDegradesToFallbackWhenStoreFails(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allowed(key) {
			t.Fatalf("Allowed(%d) = false, want true (fallback under threshold)", i)
		}
		l.Fail(key)
	}
	if l.Allowed(key) {
		t.Fatal("Allowed after max fallback failures = true, want false")
	}
}

func TestDBLimiterStoreErrorLogsWarn(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute, logger)
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	l.Allowed("k")
	out := buf.String()
	if !strings.Contains(out, "login throttle store error") || !strings.Contains(out, "op=allowed") {
		t.Fatalf("missing WARN store-error log, got: %q", out)
	}
}

func TestDBLimiterStoreErrorIncrementsCounter(t *testing.T) {
	reader := metricsdk.NewManualReader()
	otel.SetMeterProvider(metricsdk.NewMeterProvider(metricsdk.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(otel.GetMeterProvider()) })

	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	l.Allowed("k")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !hasCounter(rm, "login_throttle_store_error_count") {
		t.Fatal("login_throttle_store_error_count not recorded")
	}
}

func hasCounter(rm metricdata.ResourceMetrics, name string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}
```

Import aliases: `metricsdk "go.opentelemetry.io/otel/sdk/metric"` and `"go.opentelemetry.io/otel/sdk/metric/metricdata"`.

- [ ] **Step 2: Run the tests to confirm they FAIL**

Run: `rtk go test ./internal/auth/ -run TestDBLimiter`
Expected: FAIL — `TestDBLimiterDegradesToFallbackWhenStoreFails` fails at "Allowed after max = true" (current code fails open: closed DB → `n=0` → always allowed); the log/counter tests find nothing recorded.

- [ ] **Step 3: Add `storeError` and the degrade branches**

In `internal/auth/dblimiter.go`, add imports `context`, `go.opentelemetry.io/otel/attribute`. Add the helper and rewrite the three methods:

```go
func (l *DBLimiter) storeError(op string, err error) {
	l.errs.Add(context.Background(), 1, metric.WithAttributes(attribute.String("op", op)))
	l.logger.Warn("login throttle store error", "op", op, "err", err)
}

// Allowed reports whether key has fewer than max failures within window.
// If the store is unreachable it degrades to the in-process fallback.
func (l *DBLimiter) Allowed(key string) bool {
	cutoff := l.clk.Now().Add(-l.window)
	var n int64
	res := l.db.Model(&model.LoginAttempt{}).
		Where("key = ? AND created_at > ?", key, cutoff).
		Count(&n)
	if res.Error != nil {
		l.storeError("allowed", res.Error)
		return l.fallback.Allowed(key)
	}
	return n < int64(l.max)
}

// Fail prunes key's expired rows and records a failure at the current time.
// If the store is unreachable it records the failure in the in-process fallback.
func (l *DBLimiter) Fail(key string) {
	now := l.clk.Now()
	cutoff := now.Add(-l.window)
	l.db.Where("key = ? AND created_at <= ?", key, cutoff).Delete(&model.LoginAttempt{})
	if res := l.db.Create(&model.LoginAttempt{Key: key, CreatedAt: now}); res.Error != nil {
		l.storeError("fail", res.Error)
		l.fallback.Fail(key)
	}
}

// Reset clears all recorded failures for key.
// If the store is unreachable it clears the in-process fallback instead.
func (l *DBLimiter) Reset(key string) {
	if res := l.db.Where("key = ?", key).Delete(&model.LoginAttempt{}); res.Error != nil {
		l.storeError("reset", res.Error)
		l.fallback.Reset(key)
	}
}
```

- [ ] **Step 4: Run the tests to confirm they PASS**

Run: `rtk go test ./internal/auth/...`
Expected: PASS — all three new tests plus the three existing tests.

- [ ] **Step 5: Vet and build the whole module**

Run: `rtk go vet ./... && rtk go build ./...`
Expected: no findings; build succeeds.

- [ ] **Step 6: Commit**

```bash
rtk git add internal/auth/dblimiter.go internal/auth/dblimiter_test.go
rtk git commit -m "feat(auth): DBLimiter degrades to in-process throttle on store failure

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Post-plan (orchestrator-owned, not a task)

After both tasks pass verification: run the full suite (`rtk go test ./...`), update
`CONTEXT.md`, commit the docs, and integrate the branch. Consider adding an ADR entry
recording the degrade-to-in-process decision (extends sub-project C).

---

## Self-Review

**Spec coverage:**
- Degrade-to-in-process on store error → Task 2 (all three methods). ✓
- Internal to `DBLimiter`, `Limiter` interface unchanged → confirmed; no interface edit in either task. ✓
- Per-call detection, no circuit breaker → Task 2 branches, no state flag. ✓
- Counter `login_throttle_store_error_count` with `op` attribute from `otel.Meter("phum-panya/auth")` → Task 1 (build) + Task 2 (`storeError`, `TestDBLimiterStoreErrorIncrementsCounter`). ✓
- WARN on every store error → Task 2 `storeError` + `TestDBLimiterStoreErrorLogsWarn`. ✓
- `NewDBLimiter` gains `logger`, returns `(*DBLimiter, error)`; `newLimiter` threads logger + propagates error → Task 1. ✓
- Tests force store errors via closed connection; happy-path tests still pass → Task 2 `closedDB` + Task 1 keeps existing tests green. ✓
- Accepted trade-off (no write-back on recovery) → behavioral, needs no code; documented in spec. ✓

**Placeholder scan:** No TBD/TODO; all code steps carry full code. ✓

**Type consistency:** `NewDBLimiter(g, clk, max, window, logger) (*DBLimiter, error)` used identically in Task 1 constructor, Task 1 test helper, Task 2 tests, and `main.go`. `storeError(op string, err error)` defined once, called with `"allowed"`/`"fail"`/`"reset"`. Fallback type `*Throttle` from `NewThrottle`. ✓
