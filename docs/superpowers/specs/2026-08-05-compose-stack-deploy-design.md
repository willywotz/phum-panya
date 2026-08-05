# Compose stack deploy option (Postgres + Caddy) — design

Date: 2026-08-05 · Status: approved (design) · Relates to: [#19](https://github.com/willywotz/phum-panya/issues/19), ADR-0001, new ADR-0003

## 1. Purpose

Add a **second, optional** deploy path: a Docker Compose stack of four services —
**api**, **web**, **postgres**, **caddy** — plus a **pg_dump backup sidecar**.

This does **not** replace the default. The single Go binary with an embedded
Next.js static export and SQLite (ADR-0001) stays the **primary** path. The
compose stack is for operators who prefer containers, Postgres, and a reverse
proxy. Both paths are supported side by side. See ADR-0003, which complements
ADR-0001.

## 2. Constraints kept

- **One public origin.** Caddy serves the whole site under one domain, so the
  browser calls the API **same-origin**. This preserves the existing CSRF
  defence (`SameSite=Strict` cookie + Origin check). No CORS is added.
- **PDPA gates unchanged.** The public API still hides anything not
  `review_state = 'approved'` with consent. Media stays "public-by-URL" exactly
  as today (see §4).
- **cgo-free stays a hard rule.** The Postgres driver is pure-Go (`pgx` via
  `gorm.io/driver/postgres`). `CGO_ENABLED=0` still builds.
- **Default path untouched.** All new Go behaviour is behind config
  (`APP_DB_DRIVER`, `APP_BEHIND_PROXY`). With those unset the binary behaves
  byte-for-byte as before.

## 3. Architecture

```
                 Internet (80/443)
                        |
                   [ caddy ]  automatic HTTPS (Caddy ACME), one public origin
                    |  |  |
   /api/* ----------+  |  +---------- everything else
   (reverse_proxy)     |             (reverse_proxy)
        |        /media/* (GET/HEAD)        |
     [ api ]    file_server (shared vol)  [ web ]  Next.js standalone (node server.js)
        | GORM (postgres driver)
   [ postgres ] <---- daily pg_dump ---- [ backup sidecar ] (keep newest 14)
```

### Routing (Caddy)

| Path | Method | Target |
|---|---|---|
| `/api/*` | any | `api:8080` (reverse_proxy) |
| `/media/*` | GET, HEAD | Caddy `file_server`, shared media volume |
| everything else | any | `web:3000` (reverse_proxy) |

Only **caddy** publishes ports (80, 443). api, web, postgres are reachable only
inside the compose network.

## 4. Media served by Caddy

Today the app serves media with `engine.Static("/media", cfg.MediaDir)`
(`internal/router/router.go:100`) — a plain static file server, **no auth, no
per-request gate** (a router test asserts it is reachable without auth). The
PDPA gate is one level up: the public API **never emits URLs** for
unapproved/unconsented records, and files are content-addressed
(`ab/abcd.jpg`).

So serving `/media` reads from Caddy is **exact parity** with today's posture:

- **Reads** (`GET`/`HEAD /media/*`) → Caddy `file_server` off the shared media
  volume. Offloads photo traffic from the api.
- **Writes** (uploads: downscale, EXIF strip, JPEG/PNG/WebP allowlist) → stay on
  the api. The exact upload route is confirmed at plan time so Caddy routes only
  read methods to the file server.
- **Volume**: media mounts into **api (read-write)** and **caddy (read-only)**.
- **Path-traversal safety**: Caddy `file_server` cleans paths, matching the
  existing `/media/../secret.txt` test.

## 5. Components

Five containers. Only Caddy publishes ports.

| Service | Image / build | Publishes | Key env | Volumes |
|---|---|---|---|---|
| **caddy** | `caddy:2-alpine` + `deploy/caddy/Caddyfile` | 80, 443 | `APP_DOMAIN`, ACME email | `caddy-data` (certs), `caddy-config`, media (ro) |
| **web** | new `web/Dockerfile` — Next.js `output: standalone`, runs `node server.js` | — | `PORT=3000` | — |
| **api** | new slim `Dockerfile.api` — Go only, embeds the tracked `dist` placeholder (no Node stage) | — | `APP_DB_DRIVER=postgres`, `APP_DATABASE_URL`, `APP_BEHIND_PROXY=1`, `APP_PUBLIC_ORIGIN`, admin seed | media (rw) |
| **postgres** | `postgres:17-alpine` | — | `POSTGRES_DB/USER/PASSWORD` | `pg-data` |
| **backup** | `postgres:17-alpine` (matching `pg_dump`) + `deploy/backup/backup.sh` | — | `PGHOST=postgres`, creds | `pg-backups` |

### New files (all additive)

- `docker-compose.stack.yaml` — the 5 services + named volumes + a private network.
- `deploy/caddy/Caddyfile` — automatic HTTPS for `APP_DOMAIN`; routing per §3.
- `web/Dockerfile` — multi-stage: `npm ci && npm run build` (standalone) → slim
  Node runtime.
- `Dockerfile.api` — reuses the Go build stage, skips the web stage, embeds the
  tracked `dist` placeholder (no Node needed → small api image).
- `deploy/backup/backup.sh` — a plain loop: `pg_dump` → timestamped file in
  `/backups` → prune to newest 14 → `sleep 86400`. No cron daemon.
- `.env.example` additions — `APP_DOMAIN`, DB creds, `APP_ADMIN_PASSWORD`, etc.
- `docs/ops/deploy-compose.md` — the runbook (mirrors `deploy.md`).

### Two Next.js build modes

The default binary embeds a **static export** (`output: 'export'`). The `web`
service needs **`output: 'standalone'`**. `next.config` gates the mode on an env
flag so **both** paths keep building. The `web` service stays client-rendered
(no SSR added — ADR-0001 accepted weak SEO); the browser fetches `/api`
same-origin, so the Node server needs no internal API URL.

## 6. App-code changes (Go) — the only source touched

All test-first (§7). All portable and cgo-free.

1. **DB driver selection** — `internal/db/db.go` + `internal/config/config.go`.
   `Open` is hardwired to `glebarez/sqlite` today (`db.go:13`). Add config
   `DBDriver` (`APP_DB_DRIVER`, default `sqlite`) and `DatabaseURL`
   (`APP_DATABASE_URL`). `Open` switches on the driver: `sqlite` (unchanged,
   keeps DSN pragmas) or `postgres` via `gorm.io/driver/postgres`. `AutoMigrate`
   is already driver-agnostic; on Postgres the #15 enum `CHECK` constraints do
   apply (Postgres can `ALTER`-add them).

2. **Behind-proxy serve mode** — `internal/httpx/server.go`. Today:
   `if cfg.DevMode || cfg.Domain == "" → plain HTTP, else → autocert`
   (`server.go:32`). Add `cfg.BehindProxy` (`APP_BEHIND_PROXY=1`) +
   `cfg.PublicOrigin` (`APP_PUBLIC_ORIGIN`). When set: plain HTTP on `:8080`,
   **no autocert**, but keep Secure cookies + origin enforcement. The mode
   decision is factored into a pure function for testing.

3. **Cookie `Secure` + origin check** — `internal/auth/session.go`,
   `internal/auth/origin.go`. Make the cookie `Secure` flag (`session.go:82`)
   true under behind-proxy (public scheme is HTTPS though the api hop is HTTP).
   Feed `SameOrigin` (`origin.go:17`) the `PublicOrigin` host under behind-proxy
   so same-origin POSTs pass.

4. **Skip SQLite backup on Postgres** — `internal/backup/backup.go` + its
   scheduler. `backup.Run` uses `VACUUM INTO` (`backup.go:41`) — SQLite-only.
   Factor a `shouldRunBackup(cfg)` predicate; it is `false` when
   `DBDriver == postgres`. No Postgres backup logic enters the Go binary; the
   sidecar owns it.

**Unchanged:** API routes, auth/session model, review/consent PDPA gates, GORM
models, and the entire default SQLite binary path. `APP_DEV` and `APP_DOMAIN`
behave as before.

## 7. Testing (mandatory TDD)

Every Go change: failing test first → minimal code → green.

**Per-change (fast, no live DB):**

1. **Driver selection** — selector returns the Postgres dialector for
   `driver=postgres` and SQLite for `driver=sqlite` (compare
   `Dialector.Name()`, no connection). Config parse test for the new vars.
2. **Behind-proxy mode** — pure decision function: `BehindProxy=1` → plain HTTP,
   no autocert; `Domain` set → autocert; `Dev` → plain HTTP.
3. **Cookie + origin** — `httptest`: behind-proxy cookie has `Secure=true`;
   `SameOrigin` passes `Origin: https://<PublicOrigin>` and 403s a foreign
   origin.
4. **Backup skip** — `shouldRunBackup(cfg)` is `false` for postgres, `true` for
   sqlite.

**Integration (real Postgres):** one test runs `AutoMigrate` + a create→read
round-trip against a live Postgres, gated on `APP_DATABASE_URL`. CI gets a
`postgres` service container and a job that runs it.

**Stack acceptance (smoke, mirrors `release.yml`):**
`docker compose -f docker-compose.stack.yaml config` + `caddy validate` in CI.
Runbook documents a local smoke: stack up → `GET /api/health` → admin login →
editor→pending→approve→public → read a photo through Caddy `/media` → confirm
the pg_dump sidecar wrote a dump file.

**Regression guard:** every existing test stays green; new behaviour is behind
config, so the default SQLite binary path is unchanged.

## 8. Out of scope

- SSR / dynamic Next.js rendering (ADR-0001 accepted weak SEO).
- Making Postgres the default (it stays opt-in via `APP_DB_DRIVER`).
- Horizontal scaling / multi-replica api or web.
- A one-time SQLite→Postgres data migration tool (fresh installs only for now;
  a migration path can be a later task if a running SQLite deployment must move).
- The CD/auto-deploy mechanism for either path (#19 deploy-model decision is
  separate).
