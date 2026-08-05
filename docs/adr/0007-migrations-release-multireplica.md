# Migrations as a release step; api runs multi-replica

Status: accepted

Context: the api ran `AutoMigrate` on every process start (ADR-0001's
single-binary startup path). Sub-project D (telemetry) removed the last
per-process in-memory state — sessions, media, and throttle counters are
already stored in Postgres/Garage — so the api no longer needs to be
single-instance for correctness. But if multiple api replicas each ran
`AutoMigrate` at startup, N concurrent `ALTER TABLE`/index-create statements
would race against each other and against in-flight requests on the other
replicas, risking partial schema states or downtime during a rolling
deploy. `docs/superpowers/plans/2026-08-05-migrations-multireplica.md`
(sub-project E) makes migration a release step, decoupled from api startup,
so the api can scale to multiple replicas safely.

## Decision

- **`APP_AUTO_MIGRATE` config gate** (default `true`) controls whether
  `runServer` calls `migrateDB` on startup. The single binary and dev compose
  stack leave it at the default, so `go run ./cmd/server` and
  `docker-compose.dev.yaml` keep migrating on startup with no operator step.
- **`server migrate` subcommand** runs `migrateDB` once and exits, reusing the
  same `migrateDB(g *gorm.DB, cfg config.Config) error` helper `runServer`
  calls, so the two paths cannot drift.
- **Prod compose (`docker-compose.yaml`) runs migration as a one-shot
  `migrate` service** (`command: ["migrate"]`, `restart: on-failure`,
  `healthcheck.disable: true`) that waits on `postgres: service_healthy` and
  owns seeding the first admin (`APP_ADMIN_EMAIL`/`APP_ADMIN_PASSWORD`).
- **The `api` service sets `APP_AUTO_MIGRATE: "false"`**, drops the admin-seed
  env (the `migrate` job owns it now), and depends on
  `migrate: service_completed_successfully` — so no api replica starts until
  migration has finished exactly once.
- **`api` gets `deploy: replicas: 2`.** Dev compose is untouched and stays
  single-replica.
- **Caddy load-balances the two api replicas** for `/api/*` and `/media/*`
  using a `dynamic a` upstream (DNS-based service discovery of the `api`
  service name, refreshed every 10s), `lb_policy round_robin`, and an active
  `health_uri /api/health` check every 10s, so a failed or draining replica
  is taken out of rotation automatically. The `/traces` `forward_auth` block
  keeps its single `api:8080` target — the admin-verification sub-request
  doesn't need load-balancing.

## Why

Running migrations exactly once, before any replica serves traffic, is the
standard fix for the startup-migration-races-with-N-replicas problem, and
reusing the existing `migrateDB` helper for both the startup path and the
one-shot job avoids a second migration code path to keep in sync. Gating
startup migration behind a config flag (rather than removing it) keeps the
single-binary deployment (ADR-0001) and local dev workflow working with zero
new steps — only the prod compose stack, which already has multiple
supporting one-shot jobs (`garage-init`), gains one more. Caddy's built-in
`dynamic a` + active health check avoids introducing a separate service
registry or load balancer for two replicas behind one Docker DNS name.

## Considered options

- **Use a migration lock (advisory lock / migrate-lock table) so every
  replica can race safely.** Rejected: adds locking logic to the startup
  path for a problem a one-shot release step already solves cleanly, and
  still leaves a window where some replicas serve traffic against an
  in-progress schema change.
- **Run migrations from a CI/CD pipeline step instead of a compose
  service.** Rejected: the project's deploy model is `docker compose up
  -d --build` on a single VPS (ADR-0001, ADR-0003); a compose-native
  one-shot job keeps the same one-command deploy instead of requiring an
  external pipeline.
- **Use Caddy's static `reverse_proxy api:8080 api:8080` (two explicit
  upstreams) instead of `dynamic a`.** Rejected: `dynamic a` resolves the
  `api` service name to however many replica IPs Docker's embedded DNS
  returns, so `deploy.replicas` can change without a matching edit to the
  Caddyfile.

## Consequences

- **Prod deploy gains one step conceptually** (the `migrate` service), but
  it is fully automated inside `docker compose up -d --build` — no manual
  command required.
- **Single-binary and dev-stack deploys are unchanged**: `APP_AUTO_MIGRATE`
  defaults to `true`, so `go run ./cmd/server` and
  `docker-compose.dev.yaml` still migrate on startup exactly as before.
- **The api can now scale horizontally in prod.** Sessions, media, and
  throttle state were already externalized to Postgres/Garage (sub-project
  D and earlier), so two replicas serving traffic behind Caddy's
  round-robin/health-check upstream is safe with no further code change.
- **A stuck or failing `migrate` job blocks the api from starting** (by
  design — `service_completed_successfully` is a hard dependency), so a
  broken migration surfaces immediately instead of partially applying across
  replicas.
