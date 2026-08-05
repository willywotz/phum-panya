# Sub-project E — Migrations-as-release + multi-replica (design)

Date: 2026-08-05
Branch: `feat/multi-replica`
Status: approved design, ready for implementation plan

## Program context

Sub-project **E**, the final step of the 15-Factor + Hexagonal compliance
program (see `docs/superpowers/specs/2026-08-05-hexagonal-core-refactor-design.md`).
A (hexagonal core), B (media → Garage), C (shared throttle), and D (telemetry)
are merged. E makes schema migration a discrete release step and runs multiple
api replicas behind Caddy — the live demonstration that A–D made the api
stateless.

Order: A ✓ → B ✓ → C ✓ → D ✓ → **E** (final).

## Locked decisions (from brainstorming)

- **Config-gated auto-migrate**: `APP_AUTO_MIGRATE` (default `true`). Single
  binary / dev keep startup migration ("copy one file, run it" — ADR-0001); the
  prod stack sets `false` and runs a one-shot migrate job.
- **Prod: 2 api replicas** + a migrate job + Caddy load-balancing. **Dev stays
  single-replica** with startup auto-migrate (keeps `docker compose watch`
  hot-reload). A deliberate, justified dev/prod difference.
- **Caddy load-balances** across replicas via dynamic A-record upstreams with an
  active `/api/health` check. Exact syntax verified live.

## 1. Goal and non-goals

**Goal:** Migration becomes a discrete release step (replicas do not race DDL),
and the prod stack runs 2 api replicas behind Caddy load-balancing.

**Non-goals:**
- No business/route change.
- No change to single-binary simplicity (auto-migrate default-on there).
- Dev stays single-replica (hot reload intact).

## 2. Migration as a release step

`cmd/server/main.go` today runs `AutoMigrate` + `BackfillRecipePhotos` +
`EnsureAdmin` inside `runServer` at startup. Changes:

- Extract one helper `migrateDB(g *gorm.DB, cfg config.Config) error` =
  `AutoMigrate` → `BackfillRecipePhotos` → `EnsureAdmin` (in that order).
- New subcommand **`server migrate`** (added to the `os.Args` switch beside
  `create-admin`): opens the DB, runs `migrateDB`, logs, exits (non-zero on
  error).
- `runServer` runs `migrateDB` **only when `cfg.AutoMigrate` is true**;
  otherwise it opens the DB and serves without touching the schema.

Single-binary / dev keep the default (`true`), so a one-command run still
migrates. The prod stack sets `APP_AUTO_MIGRATE=false` and runs the migrate job
once before the replicas start.

## 3. Multi-replica (prod compose)

- **`migrate` service** (new, one-shot): the api image with `command: ["migrate"]`.
  `Dockerfile.api` has `ENTRYPOINT ["server"]`, so `command: ["migrate"]` runs
  `server migrate`. `depends_on: postgres { condition: service_healthy }`;
  `restart: on-failure`; `healthcheck: { disable: true }` (the image's
  `/api/health` healthcheck is meaningless for a one-shot that never serves).
  It carries the DB env and the **admin-seed env** (`APP_ADMIN_EMAIL` /
  `APP_ADMIN_PASSWORD`), which move here off the api. Runs `server migrate`
  (AutoMigrate + Backfill + EnsureAdmin) once, exits 0.
- **`api` service**:
  - `deploy: { replicas: 2 }` (Docker Compose v2 honors this on `up`).
  - `APP_AUTO_MIGRATE: "false"`.
  - `depends_on:` add `migrate: { condition: service_completed_successfully }`
    (keep the existing postgres/garage/garage-init conditions).
  - No `container_name` (already absent — required for replicas).
  - Admin-seed env removed (the migrate job owns it).
- **Caddy load-balancing** (`deploy/caddy/Caddyfile`): replace the single
  `reverse_proxy api:8080` in the `/api/*` and `/media/*` blocks with dynamic
  A-record upstreams so Caddy discovers both replica IPs and round-robins, with
  an active health check:

  ```caddyfile
  reverse_proxy {
      dynamic a {
          name api
          port 8080
          refresh 10s
      }
      health_uri /api/health
      health_interval 10s
      lb_policy round_robin
  }
  ```

  The `/traces` `forward_auth` may keep a single `api:8080` target (auth check,
  not load-bearing). **The exact dynamic-upstream + health-check syntax and the
  `deploy.replicas` round-robin behavior under `docker compose up` are verified
  live before committing** (Caddy modules can be version-specific).

## 4. Dev (unchanged)

Dev stays **1 api replica** with startup auto-migrate (`APP_AUTO_MIGRATE`
default `true` — no env needed). No migrate job, no Caddy LB change. This keeps
`docker compose watch` hot-reload working (multi-replica breaks single-image
live reload).

## 5. Config + docs

- New `Config.AutoMigrate bool` from `env("APP_AUTO_MIGRATE", "true") != "false"`.
- `.env.example`: a one-line note that the prod stack runs migration as a
  separate job (stack-internal default; single-binary users need not set it).
- **ADR-0007**: migrations-as-release-step + multi-replica.

## 6. Testing (TDD)

- `migrateDB`: over a temp SQLite DB, assert it creates the schema and seeds the
  admin (a `User` with the configured email + `central_admin` role exists after).
- Config: `AutoMigrate` default `true`; `false` when `APP_AUTO_MIGRATE=false`.
- The `migrate` subcommand dispatch is a thin switch case covered by the
  `migrateDB` test.
- The existing 249 tests stay green — the smoke test migrates its own DB
  directly (`db.AutoMigrate`), so it is unaffected by `runServer`'s gating.
- **Live capstone check (orchestrator):** bring up postgres + garage +
  garage-init + migrate + **2 api replicas** + Caddy, then confirm:
  (a) the migrate job ran **once** and exited 0; the api replicas did NOT
      migrate (logs show no migrate activity on the replicas);
  (b) requests through Caddy are served by **both** replicas — distinct
      `instance` (hostname) values in the JSON access logs;
  (c) stopping one replica → Caddy routes around it (health check), requests
      still succeed.

## 7. Risk / call-out

The Caddy dynamic-upstream + active-health-check syntax and whether
`deploy.replicas` round-robins cleanly under `docker compose up` (non-Swarm) are
the real risks. They are prototyped and adjusted live before the compose/Caddy
changes are committed.

## Success criteria

- `server migrate` runs the full migrate + admin-seed once; `runServer` migrates
  only when `APP_AUTO_MIGRATE=true` (default).
- The prod stack runs the migrate job once, then 2 api replicas that do NOT
  migrate; Caddy load-balances across both and health-checks them.
- Single-binary and dev are unchanged (startup auto-migrate; dev single-replica
  hot reload).
- Full test suite green. Live check confirms both replicas serve traffic and the
  migrate job ran once.
