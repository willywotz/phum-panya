# Sub-project C — Shared login throttle (multi-replica-ready) (design)

Date: 2026-08-05
Branch: `feat/shared-throttle`
Status: approved design, ready for implementation plan

## Program context

Sub-project **C** of the 15-Factor + Hexagonal compliance program (see
`docs/superpowers/specs/2026-08-05-hexagonal-core-refactor-design.md`).
A (hexagonal core) and B (media → Garage) are merged. C removes the last
per-process state so the Postgres stack becomes multi-replica-capable.

Order: A ✓ → B ✓ → **C** → D (telemetry) → E (migrations-as-release +
multi-replica Caddy load-balancing).

## Scope decision (grilled)

The A-spec's terse note said "drop SQLite (Postgres-only)". At C's design time
that was rejected: it collides with ADR-0001 (single-binary + one-file backup
is a shipping product story) and would force ~26 SQLite test files onto
Postgres. The multi-replica goal does NOT require dropping SQLite — it requires
shared state. **C is scoped to the shared throttle only; SQLite stays.**

## Locked decisions (from brainstorming)

- **Shared throttle only; keep SQLite.** Single-binary/dev/tests keep SQLite;
  the multi-replica Postgres stack gets shared state.
- **Config-selected limiter.** `APP_THROTTLE_STORE=memory` (default) | `db`,
  mirroring `APP_MEDIA_DRIVER`. Memory = the current in-memory `Throttle`
  (single process); `db` = a new DB-backed `DBLimiter` for the stack.
- **Table UNLOGGED on Postgres** (no WAL; ephemeral rate-limit data); plain
  table on SQLite.
- **Caddy load-balancing / `--scale` is sub-project E, not C.** C makes the app
  multi-replica-*capable*; E demonstrates it.

## 1. Goal and non-goals

**Goal:** Remove the in-memory login throttle's per-process state by adding a
shared, DB-backed `Limiter`, so the Postgres stack is fully stateless and ready
for multiple api replicas.

**Non-goals:**
- Do NOT drop SQLite (single-binary/ADR-0001 stays).
- Do NOT add Caddy load-balancing or `docker compose --scale` (sub-project E).
- No session change (sessions are already DB-backed and shared).
- No change to login behavior: same `max`, same `window`, same key, same
  throttle-order (Allowed → verify password → Fail/Reset).

## 2. What already satisfies multi-replica

| State | Where | Shared? |
|-------|-------|---------|
| Sessions | DB (`SessionStore`, `token_hash` looked up by query) | ✅ |
| Media | Garage object store (sub-project B) | ✅ |
| Database | Postgres in the stack | ✅ |
| **Login throttle** | **in-memory `map` per process** | ❌ ← C closes this |

## 3. The shared limiter

Add `auth.DBLimiter`, a second implementation of the existing `auth.Limiter`
port (`Allowed`/`Fail`/`Reset`), so `router.Deps.Throttle` (type `auth.Limiter`)
is unchanged. It stores one row per failed attempt and reproduces the current
sliding-window semantics exactly:

- `Allowed(key)` → `count(rows where key = ? AND created_at > now-window) < max`.
- `Fail(key)` → delete this key's rows with `created_at <= now-window` (prune),
  then insert one row at `now`.
- `Reset(key)` → delete all rows for `key` (called on successful login).

Constructor: `NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration) *DBLimiter`
— same tuning params as `NewThrottle`, plus the DB. State lives in the table,
not a process map, so replicas sharing one Postgres see each other's failures.

`*DBLimiter` satisfies `auth.Limiter` (compile-time assertion in a test).

## 4. Selection (config)

Mirror the media driver. New `Config` field `ThrottleStore` from
`APP_THROTTLE_STORE` (default `"memory"`), plus predicate:

```go
func (c Config) UsesDBThrottle() bool { return c.ThrottleStore == "db" }
```

`cmd/server/main.go` builds the limiter:

```go
var throttle auth.Limiter
if cfg.UsesDBThrottle() {
	throttle = auth.NewDBLimiter(g, clk, loginThrottleMax, loginThrottleWindow)
} else {
	throttle = auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow)
}
```

The compose stack (prod + dev) sets `APP_THROTTLE_STORE=db`. Single-binary and
dev-simple keep the in-memory throttle (zero DB writes).

## 5. Table and the UNLOGGED optimization

New GORM model in `internal/model`:

```go
// LoginAttempt is one recorded failed login, used by the DB-backed rate
// limiter so throttle state is shared across api replicas.
type LoginAttempt struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"index;not null"`
	CreatedAt time.Time `gorm:"index;not null"`
}
```

Added to `db.AutoMigrate`'s type list. GORM's default naming pluralizes the
struct to table **`login_attempts`**. On **Postgres only** (detected via
`g.Dialector.Name() == "postgres"`), after `AutoMigrate` run
`ALTER TABLE login_attempts SET UNLOGGED` — no WAL for this ephemeral data, and
losing it on a crash is harmless (the throttle simply resets to empty). On
SQLite the table is plain.

Row growth is bounded by prune-on-`Fail` per key; a key that fails then goes
silent leaves at most `max` rows until the next Postgres restart clears the
UNLOGGED table. This residue is negligible for a login limiter (one editor per
district). A periodic global sweep is deliberately out of scope (YAGNI).

## 6. Testing (TDD, mandatory)

- `DBLimiter` unit tests over a temp SQLite DB + a fake clock (the existing
  `clock` test double): `Allowed`/`Fail`/`Reset`, window pruning at the
  boundary, and a **shared-state test** — two `DBLimiter` instances on the SAME
  `*gorm.DB` (simulating two replicas): after instance A calls `Fail(key)` up to
  `max`, instance B reports `Allowed(key) == false`. Proves the shared property
  without containers.
- Compile assertion: `var _ Limiter = (*DBLimiter)(nil)`.
- Config predicate test (`UsesDBThrottle`, default memory).
- The 20 existing auth tests stay green (in-memory path untouched).
- The Postgres `UNLOGGED` `ALTER` is guarded by dialector name and only runs on
  Postgres; it is verified live against real Postgres during integration
  (orchestrator Docker check), since the unit suite runs on SQLite.

## 7. Deploy + docs

- `docker-compose.yaml` + `docker-compose.dev.yaml`: add `APP_THROTTLE_STORE: db`
  to the api `environment` block.
- **ADR-0005** "shared login throttle for multi-replica": records why the stack
  uses the DB limiter while single-binary keeps the in-memory one.
- No `.env.example` change (`APP_THROTTLE_STORE` is a stack-internal default set
  in compose, not an operator secret).

## Success criteria

- `auth.DBLimiter` implements `auth.Limiter` and reproduces the in-memory
  window/max semantics; a shared-state test passes.
- `APP_THROTTLE_STORE` selects the limiter in `main.go` (default memory); the
  compose stacks set `db`.
- `LoginAttempt` migrates on both drivers; on Postgres the table is UNLOGGED
  (verified live).
- Sessions/media/DB already shared → with `APP_THROTTLE_STORE=db` no
  per-process state remains, so the api is multi-replica-capable (demonstration
  deferred to E).
- Full test suite green; SQLite path and single-binary deploy unchanged.
