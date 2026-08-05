# Shared Login Throttle (Sub-project C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a shared, DB-backed login throttle behind the existing `auth.Limiter` port, selected by `APP_THROTTLE_STORE`, so the Postgres stack has no per-process state and is multi-replica-capable.

**Architecture:** A new `auth.DBLimiter` stores one row per failed login in a `login_attempts` table and reproduces the in-memory `Throttle`'s sliding-window rule. `main.go` picks memory (default) or db by config. SQLite stays for single-binary/dev/tests; on Postgres the table is set `UNLOGGED`.

**Tech Stack:** Go 1.26, GORM (SQLite + Postgres), gin, Docker Compose.

## Global Constraints

- TDD mandatory: failing test → confirm fail → minimal code → confirm pass → refactor.
- Do NOT drop SQLite; do NOT change login behavior (same `max=5`, `window=15m`, same key, same throttle order Allowed→verify→Fail/Reset). The in-memory `Throttle` path is unchanged.
- No Caddy load-balancing / `--scale` (that is sub-project E).
- Existing suite (236 Go tests) stays green after every task. Run `rtk go test ./...`. Build: `CGO_ENABLED=0 go build ./...`. Vet: `go vet ./...`.
- Never commit to `main`; work on branch `feat/shared-throttle` (already created).
- `*DBLimiter` must satisfy `auth.Limiter` (compile-time assertion).
- Uber Go style; American English; organized imports (`goimports`; if missing `go run golang.org/x/tools/cmd/goimports@latest -w <files>`).
- Builders do NOT run git or touch `CONTEXT.md`/docker state — the orchestrator commits after review and runs the live Postgres check.

---

## File Structure

- `internal/model/model.go` — add `LoginAttempt` entity.
- `internal/db/migrate.go` — add `LoginAttempt` to `AutoMigrate`; on Postgres set the table `UNLOGGED`.
- `internal/auth/dblimiter.go` — **new**: `DBLimiter` + `NewDBLimiter` + port assertion.
- `internal/auth/dblimiter_test.go` — **new**: window/max/reset + shared-state tests.
- `internal/config/config.go` — `ThrottleStore` field + `UsesDBThrottle()`.
- `internal/config/config_test.go` — predicate/default tests.
- `cmd/server/main.go` — `newLimiter` factory + wire into `Deps`.
- `cmd/server/main_smoke_test.go` — `newLimiter` memory-path test.
- `docker-compose.yaml`, `docker-compose.dev.yaml` — api `APP_THROTTLE_STORE=db`.
- `docs/adr/0005-shared-login-throttle.md` — **new** ADR.

---

### Task 1: `LoginAttempt` model + migrate (+ Postgres UNLOGGED)

**Files:**
- Modify: `internal/model/model.go`, `internal/db/migrate.go`
- Test: `internal/db/migrate_test.go` (**new**, `package db`)

**Interfaces:**
- Produces: `model.LoginAttempt{ ID uint; Key string; CreatedAt time.Time }`; `db.AutoMigrate` also creates `login_attempts` and (on Postgres) sets it `UNLOGGED`.

- [ ] **Step 1: Write the failing test** — `internal/db/migrate_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"
	"time"

	"phum-panya/internal/model"
)

func TestAutoMigrateCreatesLoginAttempts(t *testing.T) {
	g, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	row := model.LoginAttempt{Key: "a@x|1.2.3.4", CreatedAt: time.Now()}
	if err := g.Create(&row).Error; err != nil {
		t.Fatalf("insert LoginAttempt: %v", err)
	}
	var n int64
	if err := g.Model(&model.LoginAttempt{}).Where("key = ?", "a@x|1.2.3.4").Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./internal/db/`) — `model.LoginAttempt` undefined.

- [ ] **Step 3: Implement.** Add to `internal/model/model.go` (near the other entities):

```go
// LoginAttempt is one recorded failed login. The DB-backed rate limiter
// (auth.DBLimiter) reads these rows so throttle state is shared across api
// replicas. On Postgres the table is UNLOGGED (see internal/db.AutoMigrate).
type LoginAttempt struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"index;not null"`
	CreatedAt time.Time `gorm:"index;not null"`
}
```

(`model.go` already imports `time`.) In `internal/db/migrate.go`, add the model to the list and the Postgres UNLOGGED step:

```go
func AutoMigrate(g *gorm.DB) error {
	if err := g.AutoMigrate(
		&model.District{}, &model.User{}, &model.Session{}, &model.Doctor{},
		&model.Herb{}, &model.Recipe{}, &model.RecipePhoto{}, &model.Ingredient{}, &model.Case{},
		&model.Revision{}, &model.YearLock{}, &model.ImportBatch{}, &model.LoginAttempt{},
	); err != nil {
		return err
	}
	// login_attempts holds ephemeral rate-limit rows; skip the WAL on Postgres.
	// Losing them on a crash is harmless (the throttle just resets).
	if g.Dialector.Name() == "postgres" {
		if err := g.Exec("ALTER TABLE login_attempts SET UNLOGGED").Error; err != nil {
			return fmt.Errorf("set login_attempts unlogged: %w", err)
		}
	}
	return nil
}
```

Add `"fmt"` to `migrate.go` imports.

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./internal/db/ ./...`). On SQLite the UNLOGGED branch is skipped.

- [ ] **Step 5: Commit** — `feat(model): add LoginAttempt; migrate (UNLOGGED on Postgres)`.

---

### Task 2: `DBLimiter` (shared rate limiter)

**Files:**
- Create: `internal/auth/dblimiter.go`, `internal/auth/dblimiter_test.go`

**Interfaces:**
- Consumes: `model.LoginAttempt` (Task 1), `clock.Clock`, `auth.Limiter` (existing).
- Produces: `NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration) *DBLimiter`; `*DBLimiter` satisfies `auth.Limiter`.

- [ ] **Step 1: Write the failing tests** — `internal/auth/dblimiter_test.go`:

```go
package auth_test

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
)

func newLimiterDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "throttle.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return g
}

func TestDBLimiterAllowedThenBlockedThenWindowSlides(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l := auth.NewDBLimiter(newLimiterDB(t), fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allowed(key) {
			t.Fatalf("Allowed(%d) = false, want true", i)
		}
		l.Fail(key)
	}
	if l.Allowed(key) {
		t.Fatal("Allowed after max failures = true, want false")
	}
	fake.T = fake.T.Add(time.Minute + time.Second)
	if !l.Allowed(key) {
		t.Fatal("Allowed after window slid = false, want true")
	}
}

func TestDBLimiterResetClearsFailures(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l := auth.NewDBLimiter(newLimiterDB(t), fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		l.Fail(key)
	}
	l.Reset(key)
	if !l.Allowed(key) {
		t.Fatal("Allowed after Reset = false, want true")
	}
}

func TestDBLimiterSharedAcrossInstances(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	g := newLimiterDB(t)
	a := auth.NewDBLimiter(g, fake, 3, time.Minute)
	b := auth.NewDBLimiter(g, fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		a.Fail(key)
	}
	if b.Allowed(key) {
		t.Fatal("instance B allows after instance A hit max; throttle state is not shared")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./internal/auth/`) — `NewDBLimiter` undefined.

- [ ] **Step 3: Implement** `internal/auth/dblimiter.go`:

```go
package auth

import (
	"time"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// DBLimiter is a Limiter whose failure state lives in the database, so it is
// shared across api replicas. It reproduces Throttle's sliding-window rule:
// a key is blocked once it has max or more failures within window.
type DBLimiter struct {
	db     *gorm.DB
	clk    clock.Clock
	max    int
	window time.Duration
}

var _ Limiter = (*DBLimiter)(nil)

// NewDBLimiter returns a DB-backed Limiter.
func NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration) *DBLimiter {
	return &DBLimiter{db: g, clk: clk, max: max, window: window}
}

// Allowed reports whether key has fewer than max failures within window.
func (l *DBLimiter) Allowed(key string) bool {
	cutoff := l.clk.Now().Add(-l.window)
	var n int64
	l.db.Model(&model.LoginAttempt{}).
		Where("key = ? AND created_at > ?", key, cutoff).
		Count(&n)
	return n < int64(l.max)
}

// Fail prunes key's expired rows and records a failure at the current time.
func (l *DBLimiter) Fail(key string) {
	now := l.clk.Now()
	cutoff := now.Add(-l.window)
	l.db.Where("key = ? AND created_at <= ?", key, cutoff).Delete(&model.LoginAttempt{})
	l.db.Create(&model.LoginAttempt{Key: key, CreatedAt: now})
}

// Reset clears all recorded failures for key.
func (l *DBLimiter) Reset(key string) {
	l.db.Where("key = ?", key).Delete(&model.LoginAttempt{})
}
```

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./internal/auth/ ./...`). All three DBLimiter tests plus the existing in-memory `Throttle` tests pass.

- [ ] **Step 5: Commit** — `feat(auth): add DB-backed shared DBLimiter`.

---

### Task 3: Config — `APP_THROTTLE_STORE`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.ThrottleStore string`; `UsesDBThrottle() bool`.

- [ ] **Step 1: Write the failing test** — append to `config_test.go`:

```go
func TestThrottleStoreDefaultsToMemory(t *testing.T) {
	t.Setenv("APP_THROTTLE_STORE", "")
	c := Load()
	if c.ThrottleStore != "memory" || c.UsesDBThrottle() {
		t.Fatalf("default ThrottleStore = %q, UsesDBThrottle = %v; want memory/false", c.ThrottleStore, c.UsesDBThrottle())
	}
}

func TestThrottleStoreDBWhenSet(t *testing.T) {
	t.Setenv("APP_THROTTLE_STORE", "db")
	if !Load().UsesDBThrottle() {
		t.Fatal("UsesDBThrottle = false, want true")
	}
}
```

(The config test file is `package config` — call `Load()` unqualified, matching the existing tests in that file.)

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./internal/config/`).

- [ ] **Step 3: Implement.** Add the field to `Config`, the `Load()` line, and the predicate:

```go
	ThrottleStore string
```
```go
	ThrottleStore: env("APP_THROTTLE_STORE", "memory"),
```
```go
// UsesDBThrottle reports whether the login rate limiter stores its state in the
// database (shared across replicas) rather than in process memory.
func (c Config) UsesDBThrottle() bool { return c.ThrottleStore == "db" }
```

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./internal/config/ ./...`).

- [ ] **Step 5: Commit** — `feat(config): APP_THROTTLE_STORE selector`.

---

### Task 4: Select the limiter in `main.go`

**Files:**
- Modify: `cmd/server/main.go`
- Test: `cmd/server/main_smoke_test.go`

**Interfaces:**
- Consumes: `auth.NewThrottle`, `auth.NewDBLimiter`, `config.Config`, `clock.Clock`, `*gorm.DB`.
- Produces: `newLimiter(g *gorm.DB, cfg config.Config, clk clock.Clock) auth.Limiter`.

- [ ] **Step 1: Write the failing test** — append to `cmd/server/main_smoke_test.go` (file is `package main`):

```go
func TestNewLimiterMemory(t *testing.T) {
	l := newLimiter(nil, config.Config{ThrottleStore: "memory"}, clock.Real{})
	if _, ok := l.(*auth.Throttle); !ok {
		t.Fatalf("got %T, want *auth.Throttle", l)
	}
}
```

Ensure the test imports `config`, `clock`, `auth`.

- [ ] **Step 2: Run — expect FAIL** (`rtk go test ./cmd/server/`) — `newLimiter` undefined.

- [ ] **Step 3: Implement** in `main.go`:

```go
// newLimiter builds the login rate limiter selected by cfg.ThrottleStore:
// a shared DB-backed limiter for the multi-replica stack, or the in-process
// throttle otherwise.
func newLimiter(g *gorm.DB, cfg config.Config, clk clock.Clock) auth.Limiter {
	if cfg.UsesDBThrottle() {
		return auth.NewDBLimiter(g, clk, loginThrottleMax, loginThrottleWindow)
	}
	return auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow)
}
```

In `runServer`, replace the `Throttle:` field build. Change:

```go
		Throttle:   auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow),
```

to:

```go
		Throttle:   newLimiter(g, cfg, clk),
```

(`g` and `clk` are already in scope in `runServer` before the `Deps` literal.)

- [ ] **Step 4: Run — expect PASS** (`rtk go test ./cmd/server/ ./...` and `CGO_ENABLED=0 go build ./...`).

- [ ] **Step 5: Commit** — `feat(server): select login limiter from APP_THROTTLE_STORE`.

---

### Task 5: Compose env + ADR

**Files:**
- Modify: `docker-compose.yaml`, `docker-compose.dev.yaml`
- Create: `docs/adr/0005-shared-login-throttle.md`

- [ ] **Step 1: Add the env.** In `docker-compose.yaml`, add to the `api` service `environment:` block:

```yaml
      APP_THROTTLE_STORE: db
```

Do the SAME in `docker-compose.dev.yaml`'s dev `api` service.

- [ ] **Step 2: Write the ADR** `docs/adr/0005-shared-login-throttle.md`, following the format of `docs/adr/0004-media-object-storage-garage.md`: context (the in-memory throttle was the last per-process state blocking multiple api replicas; sessions and media are already shared), decision (a DB-backed `DBLimiter` behind the existing `auth.Limiter` port, selected by `APP_THROTTLE_STORE`, `UNLOGGED` on Postgres; the in-memory `Throttle` stays for single-binary/dev), consequences (login writes one row per failed attempt on the stack; SQLite/single-binary unchanged; actual multi-replica load-balancing is sub-project E).

- [ ] **Step 3: Verify**

Run (with the stack's required vars exported inline):
```bash
GARAGE_RPC_SECRET=x APP_S3_ACCESS_KEY=k APP_S3_SECRET_KEY=s APP_DOMAIN=example.org APP_ADMIN_PASSWORD=p POSTGRES_PASSWORD=pp docker compose -f docker-compose.yaml config -q && echo PROD_OK
GARAGE_RPC_SECRET=x APP_S3_ACCESS_KEY=k APP_S3_SECRET_KEY=s POSTGRES_PASSWORD=pp docker compose -f docker-compose.dev.yaml config -q && echo DEV_OK
```
Expected: both print OK. Confirm `docker compose -f docker-compose.yaml config | grep APP_THROTTLE_STORE` shows `db` on the api service.

- [ ] **Step 4: Commit** — `feat(deploy): stack uses the shared DB throttle (ADR-0005)`.

- [ ] **Step 5 (orchestrator):** live Postgres check that `AutoMigrate` sets `login_attempts` UNLOGGED and the DBLimiter shares state across two connections; then update `CONTEXT.md` and run the final whole-branch review.

---

## Self-Review

**Spec coverage:**
- Shared DB-backed limiter behind `auth.Limiter` → Task 2.
- `LoginAttempt` table + Postgres UNLOGGED → Task 1.
- Config selector `APP_THROTTLE_STORE` (memory default) → Task 3.
- `main.go` selection → Task 4.
- Compose stacks set `db` + ADR-0005 → Task 5.
- Sessions already shared (no change) — noted in the design; no task needed.
- No SQLite drop, no login-behavior change, no Caddy `--scale` — held across all tasks.

**Placeholder scan:** The ADR body (Task 5 Step 2) is described, not templated verbatim, but the required sections and content are specified; every code/YAML block is complete. The orchestrator's live Postgres UNLOGGED check is a deliberate integration step (SQLite unit tests cannot exercise it), not a placeholder.

**Type consistency:** `model.LoginAttempt{ID,Key,CreatedAt}` is used identically in Tasks 1 and 2; table name `login_attempts` (GORM default plural) is used in the migrate ALTER and the tests' queries. `NewDBLimiter(g, clk, max, window)` matches its call in `newLimiter` (Task 4). `UsesDBThrottle()`/`ThrottleStore` are consistent across Tasks 3 and 4. `newLimiter(g, cfg, clk) auth.Limiter` matches the `Deps.Throttle` field type (`auth.Limiter`).
