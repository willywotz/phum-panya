# Optional container stack: split services, Postgres, Caddy

Status: accepted

Context: a second deploy option requested for operators who prefer containers
(`docs/superpowers/specs/2026-08-05-compose-stack-deploy-design.md`; relates to
[#19](https://github.com/willywotz/phum-panya/issues/19)). **This ADR
complements ADR-0001; it does not replace it.**

## Decision

We add a **second, optional** deploy path as a Docker Compose stack of four
services — **api** (Go), **web** (Next.js `output: standalone`), **postgres**,
**caddy** — plus a **pg_dump backup sidecar**. Caddy terminates TLS (its own
ACME), serves one public origin, reverse-proxies `/api/*` to the api and
everything else to the web service, and serves `/media/*` reads directly with
its `file_server`. The api talks to Postgres through the existing GORM layer via
the pure-Go `gorm.io/driver/postgres`, selected at runtime by
`APP_DB_DRIVER=postgres`. The api runs plain HTTP behind Caddy in a new
"behind-proxy" mode (`APP_BEHIND_PROXY=1` + `APP_PUBLIC_ORIGIN`): no built-in
autocert, but Secure cookies and same-origin enforcement stay on.

**The single Go binary + embedded static export + SQLite (ADR-0001) remains the
default and primary path.** The compose stack is opt-in. All new Go behaviour is
behind config, so the default path is unchanged when that config is unset.

## Why

The owner-run single-VPS model (ADR-0001) is the right default and stays. But
some operators run Docker already and prefer Postgres, container isolation, and
a reverse proxy. ADR-0001 kept GORM portability discipline precisely so Postgres
is a driver swap, not a rewrite — so offering this second path is cheap and does
not compromise the default. Caddy's automatic HTTPS and `file_server` remove the
need for the api to own TLS or media serving in this topology.

## Considered options

- **Supersede ADR-0001 (make the container stack the only path).** Rejected: the
  non-IT owner on a ~350 THB/month VPS still wants one file + one data folder to
  run and back up. The binary must stay the default.
- **Split into api + web but keep SQLite (no Postgres, no Caddy).** Rejected: the
  request is specifically a container-native stack with Postgres and a reverse
  proxy; a shared SQLite file across separate containers is fragile.
- **Separate subdomains for api and web (CORS).** Rejected: the CSRF defence is a
  `SameSite=Strict` cookie + Origin check; one public origin behind Caddy keeps
  that intact with no CORS.

## Consequences

- **Two Next.js build modes.** The default binary embeds `output: 'export'`; the
  web service uses `output: 'standalone'`. `next.config` gates the mode on an env
  flag so both keep building.
- **Backups differ per path.** SQLite path: the in-app `VACUUM INTO` loop.
  Postgres path: the app skips that loop; the pg_dump sidecar owns backups
  (newest 14 kept), mirroring the SQLite behaviour.
- **Media is public-by-URL, unchanged.** Caddy serving `/media` reads is exact
  parity with the current unauthenticated `engine.Static` handler; the PDPA gate
  stays at the API (URLs for unapproved/unconsented records are never emitted).
- **cgo-free preserved.** `gorm.io/driver/postgres` (pgx) is pure Go;
  `CGO_ENABLED=0` still builds.
- **No data migration tool yet.** Fresh Postgres installs only; moving a running
  SQLite deployment to Postgres is a later task if needed.
- **CD is still separate.** This ADR defines the topology, not the auto-deploy
  mechanism (#19).
