# Shared login throttle: DB-backed `DBLimiter` behind `auth.Limiter`

Status: accepted

Context: sessions (Postgres-backed) and media (Garage, ADR-0004) are already
shared backing services, but the login rate limiter (`internal/auth.Throttle`)
still kept its failed-attempt counters in an in-process map — the last
per-process state left in the api (15-Factor: processes must be stateless).
That in-memory state is harmless for a single api instance, but it silently
breaks the login throttle once more than one api replica sits behind the
load balancer: each replica tracks its own attempt count, so an attacker
gets `max` attempts per replica instead of `max` attempts total.
Sub-project D (`docs/superpowers/plans/2026-08-05-shared-throttle.md`) adds a
DB-backed limiter that shares state across replicas while keeping the
existing `auth.Limiter` port and login behavior unchanged.

## Decision

We add `DBLimiter` (`internal/auth/dblimiter.go`), a `*gorm.DB`-backed
implementation of the existing `auth.Limiter` port, alongside the current
in-memory `Throttle`. Both satisfy the same port, so `internal/handler`'s
login flow (`Allowed` → verify → `Fail`/`Reset`, `max=5`, `window=15m`) does
not change. `DBLimiter` stores one row per failed attempt in a new
`login_attempts` table (`internal/model.LoginAttempt`); on Postgres,
`internal/db`'s `AutoMigrate` sets that table `UNLOGGED` — attempt counters
are disposable rate-limit bookkeeping, not data that needs WAL durability or
replication, so skipping the write-ahead log trades a small crash-recovery
gap (rows since the last checkpoint) for less I/O on every failed login. A
new `APP_THROTTLE_STORE` config field (`memory` default, `db` opt-in) picks
the implementation at startup via `cmd/server/main.go`'s `newLimiter`
factory; the in-memory `Throttle` stays as the default for the single-binary
and dev-without-Postgres cases. Both compose stacks
(`docker-compose.yaml`, `docker-compose.dev.yaml`) set
`APP_THROTTLE_STORE: db` on the api service, since both already require
Postgres.

## Why

The `auth.Limiter` port (hexagonal architecture) already isolated the login
handler from the throttle's storage; adding a second adapter behind it is a
driver swap, following the same discipline as ADR-0003 (Postgres) and
ADR-0004 (Garage). A DB-backed limiter reuses the Postgres instance the
stack already runs — no new backing service, no new operational surface —
and matches how sessions already share state across replicas. `UNLOGGED` is
chosen over a normal table because login-attempt counters are short-lived
(`window=15m`) and reconstructible: losing them on a crash only means a
temporary reset of an attacker's or a legitimate user's attempt count, never
data loss that matters.

## Considered options

- **Move the throttle into Redis/another cache service.** Rejected: adds a
  new backing service and operational surface (deploy, backup, monitoring)
  for a problem Postgres already solves; the single-VPS deploy model
  (ADR-0001, ADR-0003) favors reusing existing services over adding new
  ones.
- **Make `DBLimiter` the only implementation, drop the in-memory `Throttle`.**
  Rejected: the plan's global constraint keeps SQLite/single-binary
  deployments working without a live Postgres connection; the in-memory
  path remains the default and is unchanged.
- **Leave the table as a normal (logged) table.** Rejected: every failed
  login would incur WAL writes for data that is disposable and
  short-lived; `UNLOGGED` is the documented Postgres mechanism for exactly
  this case.

## Consequences

- **One row per failed login on the Postgres stack.** `docker-compose.yaml`
  and `docker-compose.dev.yaml` now default the api to `APP_THROTTLE_STORE:
  db`; every failed login attempt writes a `login_attempts` row instead of
  updating an in-process counter.
- **SQLite/single-binary deployments are unchanged.** `APP_THROTTLE_STORE`
  defaults to `memory`, so a single api process without Postgres keeps the
  existing in-memory `Throttle` behavior exactly as before.
- **Multi-replica load-balancing is out of scope here.** This ADR only
  makes the throttle's storage shareable; actually running more than one
  api replica behind Caddy (`--scale`, load-balancing config) is
  sub-project E.
