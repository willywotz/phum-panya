# Dev compose — web + api split behind nginx (design)

**Date:** 2026-08-04
**Status:** approved

## Goal

Give developers a hot-reload dev stack that runs the Next.js frontend and the Go
API as **separate** containers behind an **nginx** reverse proxy on a single
origin. Production is unchanged (one embedded binary via `docker-compose.yaml`).

## Why this shape

- The frontend already calls the API with **relative** URLs (`fetch('/api/...')`
  in `web/lib/api.ts`) and reads uploads at `/media`. An nginx **same-origin**
  proxy routes `/api` and `/media` to the Go service and everything else to the
  Next.js dev server, so **no frontend code changes** are needed.
- In dev (`APP_DEV=1`, no `APP_DOMAIN`) the Go `SameOrigin` CSRF check is a no-op
  and the session cookie is not `Secure`, so cross-service auth works over plain
  HTTP behind the proxy.
- The Go server embeds a tracked placeholder `internal/webui/dist/index.html`, so
  it compiles and serves the API without a UI build. nginx never routes page
  requests to it, so the placeholder is unused. **No Go changes needed.**

## Architecture

```
        host:${APP_HOST_PORT:-8080}
                 │
             ┌───▼────┐   /api/  /media/   ┌──────────────┐
             │ nginx  ├────────────────────► api  (Go)     │  air live-reload
  browser ──►│ :80    │                     │ :8080        │  APP_DEV=1
             │        ├──── everything ─────► web  (Next)   │  next dev
             └────────┘   else + HMR ws     │ :3000        │  HMR
                                            └──────────────┘
```

Only **nginx** is published to the host. `web` and `api` are reachable only on
the compose network — all traffic goes through the proxy, so the dev stack is a
true same-origin integration test of the API contract and auth behavior.

## Components

New file `docker-compose.dev.yaml` with three services; prod
`docker-compose.yaml` is untouched.

### nginx (`deploy/nginx/dev.conf`)
- `nginx:1.27-alpine`, conf bind-mounted read-only, published `${APP_HOST_PORT:-8080}:80`.
- `location /api/` and `location /media/` → `http://api:8080` (full URI preserved).
- `location /` → `http://web:3000`, with `Upgrade`/`Connection` set so the
  `/_next/webpack-hmr` WebSocket tunnels through for hot-module-reload.

### api (Go)
- `golang:1.25-alpine`, repo bind-mounted at `/app`.
- Installs pinned **air `v1.67.0`** (newest release compatible with the project's
  Go 1.25; v1.67.4+ requires Go 1.26) into the cached `/go` volume (only if absent),
  then runs `air -c .air.toml`, which rebuilds `./cmd/server` on any `*.go` change.
- Env: `APP_DEV=1`, `APP_HTTP_ADDR=:8080`, admin bootstrap
  (`APP_ADMIN_EMAIL`/`APP_ADMIN_PASSWORD`), `/data` paths, `GOCACHE=/go/.cache`.
- Volumes: repo source, named `dev-go-cache` (`/go`, module + build cache), named
  `dev-data` (`/data`: SQLite DB, media, backups). Air builds to `/tmp/air`
  (container-only) so the host repo stays clean.

### web (Next.js)
- `node:24-alpine`, `./web` bind-mounted, with named volumes shadowing
  `/app/web/node_modules` and `/app/web/.next` so the host does not clobber the
  container install.
- Installs deps once (`npm ci` if `node_modules/.bin/next` is missing), then runs
  `next dev -H 0.0.0.0 -p 3000`.
- `WATCHPACK_POLLING=true` for reliable HMR over bind mounts on WSL2/Docker.

## Data flow

1. Browser → `http://localhost:${APP_HOST_PORT}` → nginx.
2. Page/`/_next/*`/HMR → `web` (`next dev`), rendered/hot-reloaded live.
3. `web` code runs `fetch('/api/...')` / loads `/media/...` on the same origin →
   nginx → `api` (Go), which reads/writes the `dev-data` volume.

## Constraints honored

- **15-Factor III (config):** all service config via env; no secrets baked in.
- **15-Factor X (dev/prod parity) — deviation, documented:** dev is three
  containers, prod is one binary. The nginx same-origin layout keeps the API
  contract, relative-URL calls, and cookie/CSRF behavior identical to prod, which
  is the parity that matters. Accepted for hot-reload dev ergonomics.
- No app-code changes; backend/API/data model untouched.
- No CDN; base images pulled from the registry, deps vendored via npm/Go modules.

## Verification

`APP_ADMIN_PASSWORD=… docker compose -f docker-compose.dev.yaml up`, then, through
nginx on one origin:
- `GET /` returns the Next.js dev landing page.
- `GET /api/health` returns `{"status":"ok"}` from the Go service.
- editing a `web/**` file hot-reloads; editing a `*.go` file triggers an air rebuild.

## Out of scope

- TLS (dev is plain HTTP; prod TLS is the single-binary path).
- Serving the built static export in dev (that is the prod binary's job).
- Publishing `web`/`api` to the host (nginx-only by design).
