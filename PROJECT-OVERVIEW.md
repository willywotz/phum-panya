# phum-panya — Full Project Overview

*A single, detailed picture of the whole project. Written in Simplified
Technical English. Last built from the repository on 2026-08-09.*

For the day-to-day work log, read `CONTEXT.md`. For the operator hand-off,
read `docs/HANDOFF.md`. This file gives the complete map in one place.

---

## 1. What the project is

phum-panya is a web app that keeps folk-medicine records (ตำรายาหมอพื้นบ้าน)
for one Thai province. It groups all data by district (อำเภอ).

- The **public** can read consented healer (หมอพื้นบ้าน) profiles, their
  recipes (ตำรับยา), and treatment cases (เคส).
- **Staff** write the data. There is **one central admin** and **one editor per
  district**.
- Users open the site by QR-code scan. There is **no mobile-store app** and
  **no map**.
- The reference app is a Thai "Tok Bidan" herbal app, but without the map.

The client chat log is in `messages.txt`.

### Shape of the product

The app ships as **one self-hosted Go binary**. The binary holds:

- The HTTP API (Go, Gin + GORM).
- The full web UI (Next.js 15, exported to static files, embedded with
  `embed.FS`).
- A pure-Go SQLite driver, so the binary needs **no cgo** and **no external
  database** for the default path.

You copy one file to a host and run it. It can also run as a bigger container
stack (Postgres + Caddy + object storage) when an operator wants to scale. Both
paths run the same Go code.

---

## 2. How to build, run, and test

### Build

```
make build
```

`make build` runs the Next.js static export, copies it into
`internal/webui/dist/`, then builds `./server` with `CGO_ENABLED=0`. You need
Go and Node installed. Nothing else. The result is one static, cgo-free binary.

Cross-compiled release binaries:

```
make build-release   # bin/server-linux-amd64 + bin/server.exe
```

### Run

Configure with environment variables (all optional, sane defaults — see
§7). Minimal dev run:

```
APP_DEV=1 APP_ADMIN_EMAIL=admin@example.com APP_ADMIN_PASSWORD=changeme ./server
```

On first run the server seeds the first central admin from `APP_ADMIN_EMAIL`
and `APP_ADMIN_PASSWORD`, but only when no admin exists yet.

### Subcommands

The `cmd/server` binary has these subcommands (see `cmd/server/main.go`):

| Command | Action |
|---|---|
| `server` (or `server run`) | Run in the foreground / under the service manager |
| `server migrate` | Run the schema migration + backfill + admin seed once, then exit |
| `server create-admin` | Seed the first central admin, then exit |
| `server service install` | Register the OS service (Windows SCM / systemd / launchd) |
| `server service uninstall\|start\|stop\|restart` | Control the OS service |

`service install` takes `--admin-email`, `--admin-password`, `--domain`, and
`--http-addr` flags. It bakes them into the service environment. Data lives
under `%ProgramData%\phum-panya` on Windows or `/var/lib/phum-panya` elsewhere.

### Windows MSI

A release ships a `phum-panya-<version>-windows-amd64.msi`. The WiX wizard asks
for the admin email, admin password, and public domain. It installs
`server.exe`, then registers and starts the `phum-panya` service.

### Test

```
go test ./...                # Go unit + integration tests (265+ tests)
cd web && npx playwright test # end-to-end browser tests (~19 specs)
```

---

## 3. Technology stack

| Layer | Choice |
|---|---|
| Language (backend) | Go 1.26 (`go.mod` `go 1.26.0`, `toolchain go1.26.5`) |
| HTTP framework | Gin `v1.12.0` |
| ORM | GORM `v1.31.2` |
| Default database | SQLite via pure-Go `glebarez/sqlite` (cgo-free) |
| Scale database | PostgreSQL via pure-Go `gorm.io/driver/postgres` |
| TLS (single binary) | Go-native ACME / Let's Encrypt (`autocert`) |
| Image processing | `disintegration/imaging` + `golang.org/x/image` |
| Excel import/export | `xuri/excelize/v2` |
| Object storage client | `aws-sdk-go-v2` (S3 path-style, for Garage) |
| OS service wrapper | `kardianos/service` |
| Telemetry | OpenTelemetry SDK + Prometheus exporter + `otelgin` + GORM tracing |
| Frontend | Next.js 15.5 (static export), React, TypeScript |
| Styling | Tailwind CSS v4 + shadcn/ui (Radix), `next-themes` light/dark |
| Font | Sarabun (`@fontsource/sarabun`, weights 300/500/700), vendored, no CDN |
| Password hash | bcrypt (`golang.org/x/crypto`) |
| E2E tests | Playwright |

The codebase is about **117 Go files** and **65 TypeScript files** (plus YAML,
CSS, TOML).

---

## 4. Architecture

The project follows **Hexagonal architecture** (ports and adapters) and the
**15-Factor App** method. These are project rules, not optional.

### 4.1 Layers

```
                         ┌─────────────────────────────┐
   HTTP request  ───────>│  Gin engine (internal/router)│
                         │  middleware: recovery, otel,  │
                         │  access log, metrics, CSRF    │
                         └──────────────┬───────────────┘
                                        │ calls
                  ┌─────────────────────▼─────────────────────┐
                  │  Feature packages (handlers + ports)        │
                  │  auth, district, user, herb, doctor,        │
                  │  recipe, caserec, review, yearlock,         │
                  │  importer, publicapi, export, backup        │
                  └─────────────────────┬─────────────────────┘
                          depends on a PORT interface, never *gorm.DB
                  ┌─────────────────────▼─────────────────────┐
                  │  Adapters: GORM Repo, media.Store,          │
                  │  auth.Sessions/Limiter/Users                │
                  └─────────────────────┬─────────────────────┘
                  ┌─────────────────────▼─────────────────────┐
                  │  internal/model — PURE domain entities      │
                  │  (imports only "time"; gorm tags are        │
                  │  inert strings)                             │
                  └───────────────────────────────────────────┘
```

Key rules that the hexagonal refactor (sub-project A) locked in:

1. `internal/model` is a **pure** domain package. It imports only `time`.
   `AutoMigrate` and the backfill live in `internal/db`, not in `model`.
2. Every HTTP handler depends on a **port interface**, never on `*gorm.DB` or a
   concrete `*Repo`. Each feature package declares a `repository` port that its
   GORM `Repo` satisfies. You can see the ports in the `*/port_test.go` files.
3. `media.Store` is an interface. The filesystem version is `LocalStore`; the
   object-storage version is `S3Store`. `auth.Limiter`, `auth.Sessions`, and
   `auth.Users` are also ports.

### 4.2 The composition root

`internal/router/router.go` (`NewEngine`) is the composition root. It builds
every adapter from `Deps` and wires every module's routes. `router` cannot live
inside `httpx`, because every module imports `httpx` for `httpx.OK` / `httpx.Err`
— a back-import would make a cycle.

`cmd/server/main.go` builds `Deps` from `config.Load()` and hands them to
`NewEngine`. It also picks the adapters by config: `newLimiter` (memory or DB),
`newMediaStore` (local or S3).

### 4.3 Package map (`internal/`)

| Package | Job |
|---|---|
| `model` | Pure domain entities (District, User, Doctor, Herb, Recipe, RecipePhoto, Ingredient, Case, Revision, YearLock, ImportBatch, LoginAttempt, Session) |
| `config` | Reads all `APP_*` env vars into a `Config` struct; derives `UseTLS`, `CookieSecure`, `AllowedOriginHost`, `BackupEnabled`, driver switches |
| `db` | Opens SQLite or Postgres; `AutoMigrate`; `BackfillRecipePhotos`; dialector helpers |
| `httpx` | Small HTTP helpers: `OK`, `Err`, JSON, `ServeContext` (context-aware serve + autocert TLS) |
| `router` | The composition root; assembles the full `gin.Engine` |
| `clock` | `Clock` interface + `Real` clock; lets tests control time |
| `auth` | Sessions, login handler, bcrypt password, session store, same-origin CSRF, role middleware, in-memory `Throttle` and DB `DBLimiter` |
| `bootstrap` | `EnsureAdmin` — seeds the first central admin (no-op when creds empty) |
| `district` | District CRUD (admin writes, editor reads) |
| `user` | Staff user CRUD, activate, set-password (admin only) |
| `herb` | Shared herb catalog; add/edit-own, admin merge/alias, near-duplicate lookup, reconcile pending |
| `doctor` | Healer profiles; photo, unpublish, review workflow |
| `recipe` | Formulas; multi-photo child rows; year-lock guard |
| `caserec` | Anonymous treatment cases; year-lock guard |
| `revision` | Append-only audit log (`Append` runs after the write tx commits) |
| `review` | Approval queue: queue, entry detail, approve, reject, approve whole doctor tree |
| `yearlock` | Freeze/unfreeze a `data_year`; `IsLocked` guard |
| `importer` | Bulk Excel import: 4-sheet template → parse → dry-run report → commit → per-batch undo |
| `publicapi` | Public read API (consent + `approved` gated; hides private fields) |
| `export` | Staff CSV / XLSX export |
| `backup` | Nightly single-zip backup (DB + media) + restore; `POST /api/backup/run` |
| `media` | `Store` port; `LocalStore` (filesystem) and `S3Store` (Garage); image downscale + EXIF strip; content-type sniff (JPEG/PNG/WebP only) |
| `telemetry` | slog JSON logs, OTel metrics (`/metrics`), OTel traces, `AccessLog`, `InstrumentDB` |
| `svc` | `kardianos/service` wrapper; `Run`, `Control` for OS service manager |
| `webui` | Embeds `dist/` with `embed.FS`; serves the SPA + assets; JSON 404 for `/api/*` |

---

## 5. Data model

Nine core records plus support tables. See `internal/model/model.go`.

```
District ──< Doctor ──< Recipe ──< Case
                          │          
                          ├──< RecipePhoto
                          └──< Ingredient >── Herb (shared catalog)
```

- **District** (อำเภอ) — the grouping unit. Name + province. A fixed list; it
  never cascades a delete onto its doctors.
- **User** — a staff login. Role is `central_admin` or `district_editor`.
  A district editor has a `DistrictID`. `PasswordHash` is never sent to JSON.
- **Session** — server-side login session keyed by a hashed token.
- **Doctor** (หมอพื้นบ้าน) — the healer profile. One row per healer, with
  `FirstYear`. Carries `Code` for reliable paper-form linking. Needs
  `ConsentObtained = true` before the public can see it.
- **Herb** (สมุนไพร) — ONE shared province-wide catalog. P5 adds
  `CreatedByDistrictID` (provenance) and `AliasOfID` (merge/alias target).
- **Recipe** (ตำรับยา) — a formula that belongs to one doctor. Carries `Code`
  and `DataYear`. Holds many `RecipePhoto` child rows (issue #18 replaced the
  single photo string).
- **Ingredient** — one row inside a recipe. Exactly one of `HerbID` or
  `PendingHerbName` is set (a `chk_herb_xor` check enforces this). The
  pending-herb name lets district entry continue when a herb is not in the
  catalog yet.
- **Case** (เคส) — an anonymous treatment result linked to one recipe. The
  patient is anonymous (gender range + age range only).
- **Revision** — append-only audit log. One snapshot per approved change (plus
  reject events): entity type + id → who / when / action / after-JSON.
- **YearLock** — freezes one `data_year` (primary key) read-only.
- **ImportBatch** — one bulk-import run, so an admin can undo it. Doctor,
  Recipe, and Case carry a `BatchID`.
- **LoginAttempt** — one recorded failed login, read by the DB-backed rate
  limiter so throttle state is shared across replicas. On Postgres this table
  is `UNLOGGED`.

### Governance columns

Doctor, Recipe, and Case share the approval workflow columns:

- `ReviewState` — `approved` | `pending` | `rejected` (a check constraint).
- `PendingJSON` — the staged edit overlay.
- `PendingPhoto` — a staged photo change (Doctor and Case).
- `PendingDelete` — a queued delete.
- `RejectionReason` — why an admin rejected the change.

---

## 6. HTTP API surface

All routes use **full-English names**, never short aliases (a project rule).
Example: `/api/current-user`, not `/api/me`. All `/api/*` routes carry
`auth.LoadUser`, so the current user is attached when a session cookie is
present.

### Public (no auth)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | Liveness check |
| GET | `/api/public/districts` | List districts |
| GET | `/api/public/doctors` | List consented, approved healers (`?district_id=`, `?q=`) |
| GET | `/api/public/doctors/:id` | One healer detail |
| GET | `/api/public/herbs` | List herbs (aliases filtered out) |
| GET | `/api/public/recipes` | List recipes (`?district_id=`, `?herb_id=`, `?q=`) |
| GET | `/media/*key` | Stream an uploaded image (through the media adapter) |

### Auth

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/login` | Log in (bcrypt + throttle) |
| POST | `/api/logout` | Log out |
| GET | `/api/current-user` | Current user (returns `full_name`) |
| GET | `/api/authorization/verify-admin` | Caddy forward-auth target that gates `/traces` to central-admin |

### Staff — records (require auth; editor scoped to own district)

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/api/districts`, PUT/DELETE `/:id` | District CRUD (writes admin-only) |
| GET/POST | `/api/users`, PUT `/:id`, POST `/:id/active`, `/:id/password` | User admin (admin-only) |
| GET/POST | `/api/herbs`, PUT/DELETE `/:id` | Herb catalog (add/edit-own for editors) |
| GET | `/api/herbs/near-duplicates?thaiName=` | Save-time duplicate nudge |
| GET | `/api/herbs/pending`, POST `/api/herbs/reconcile` | Pending-herb tools (admin) |
| POST | `/api/herbs/:id/merge/:canonicalId` | Merge/alias (admin) |
| GET | `/api/storage` | Media storage meter (admin) |
| GET/POST | `/api/doctors`, PUT/DELETE `/:id` | Doctor CRUD |
| POST | `/api/doctors/:id/photo`, `/:id/unpublish` | Photo change, unpublish |
| GET/POST | `/api/recipes`, PUT/DELETE `/:id` | Recipe CRUD |
| POST | `/api/recipes/:id/photo` | Append a recipe photo |
| GET | `/api/recipes/resolve-doctor` | Resolve a doctor by code |
| GET/POST | `/api/cases`, PUT/DELETE `/:id`, POST `/:id/photo` | Case CRUD + photo |

### Staff — governance (central-admin only)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/review/queue` | Pending approval queue |
| GET | `/api/review/entry/:entityType/:entityId` | Current-vs-proposed detail |
| POST | `/api/review/entry/:entityType/:entityId/approve` | Approve one change |
| POST | `/api/review/entry/:entityType/:entityId/reject` | Reject with a reason |
| POST | `/api/review/doctor/:doctorId/approve-all` | Approve a whole doctor tree |
| GET/POST | `/api/year-locks`, DELETE `/:dataYear` | Lock / unlock a data year |
| POST | `/api/imports?dryRun=true\|false` | Upload `.xlsx`; dry-run report or commit |
| POST | `/api/imports/:batchId/undo` | Undo a bulk-import batch |
| POST | `/api/backup/run` | Run a backup now |
| GET | `/api/export/doctors.csv`, `.xlsx` (and more) | Bulk export |

### Internal (not routed publicly by Caddy)

| Method | Path | Purpose |
|---|---|---|
| GET | `/metrics` | Prometheus metrics |

---

## 7. Configuration (environment variables)

All config comes from `APP_*` env vars (15-Factor III). Defaults keep the
single-binary path zero-config. See `internal/config/config.go`.

| Variable | Default | Purpose |
|---|---|---|
| `APP_HTTP_ADDR` | `:8080` | Listen address |
| `APP_DOMAIN` | unset | Public domain; enables built-in TLS when set (and not dev / not behind proxy) |
| `APP_DEV` | unset | `1` = plain HTTP, no TLS |
| `APP_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `APP_DB_PATH` | `data/app.db` | SQLite file path |
| `APP_DATABASE_URL` | unset | Postgres DSN |
| `APP_AUTO_MIGRATE` | `true` | Migrate at startup; the stack sets `false` and runs a one-shot `migrate` job |
| `APP_BEHIND_PROXY` | unset | `1` = trust a reverse proxy (Caddy) |
| `APP_PUBLIC_ORIGIN` | unset | Public origin URL; drives CSRF host + cookie Secure |
| `APP_MEDIA_DIR` | `data/media` | Local media directory |
| `APP_MEDIA_DRIVER` | `local` | `local` or `s3` (Garage) |
| `APP_S3_ENDPOINT/REGION/BUCKET` | region `garage`, bucket `media` | Object storage target |
| `APP_S3_ACCESS_KEY/SECRET_KEY` | unset | Garage key: access = `GK`+24 hex, secret = 64 hex |
| `APP_S3_USE_PATH_STYLE` | `true` | Path-style S3 addressing |
| `APP_THROTTLE_STORE` | `memory` | `memory` (per-process) or `db` (shared) login throttle |
| `APP_BACKUP_DIR` | `data/backup` | Nightly zip output (SQLite path only) |
| `APP_ADMIN_EMAIL/PASSWORD` | unset | First central admin seed |
| `APP_LOG_LEVEL/FORMAT` | `info` / `json` | slog level and format |
| `APP_OTLP_ENDPOINT` | unset | OTLP gRPC target (traces); degrades gracefully when unset |
| `APP_SERVICE_NAME` | `phum-panya-api` | Telemetry service name |

Config derivations worth knowing:

- `UseTLS()` — true only when not dev, not behind proxy, and a domain is set.
- `CookieSecure()` — true under production TLS, or behind a proxy when the
  public origin is HTTPS. So a dev stack over plain HTTP still gets a usable
  cookie.
- `BackupEnabled()` — false on Postgres (the `pg_dump` sidecar owns backups).

---

## 8. Frontend (`web/`)

Next.js 15 App Router, TypeScript, exported to static HTML (no Node server for
the single-binary path). Two route groups:

- `(public)` — landing page, `doctors`, `doctor`, `herbs`, `recipes`. Public
  read-only browsing.
- `(staff)` — `login`, then `staff/*`: `cases`, `districts`, `doctors`,
  `herbs`, `imports`, `recipes`, `review`, `users`, `year-locks`.

Design and shared pieces:

- **UI kit**: shadcn/ui (Radix) under `web/components/ui/*`. Shared building
  blocks: `CrudForm`, `CrudTable`, `IngredientEditor`, `PhotoUpload`,
  `HerbAddForm`, `ExportLinks`.
- **Theme**: warm herbal theme with a light/dark toggle (`next-themes`).
- **Font**: Sarabun, vendored (Thai subset bundled, no CDN). Body weight 300,
  titles weight 500.
- **i18n**: bilingual — Thai default, English label toggle. Content itself is
  not translated. See `web/lib/i18n.tsx`.
- **API client**: `web/lib/api.ts` uses relative `fetch('/api/...')`, so
  same-origin proxying works. `api.upload` handles multipart.
- **Auth/role gating**: `web/lib/auth.tsx`. Staff nav shows admin-only links
  (`review`, `year-locks`, `imports`, `districts`, `users`) only to a central
  admin. `RequireAdmin` bounces a district editor off an admin URL.
- **Dev build**: Turbopack (`next dev --turbopack`). Production build stays
  webpack (`export` or `standalone`).

E2E tests live in `web/e2e/*.spec.ts` (Playwright). The suite runs serially
(`workers: 1`) because the shared dev SQLite DB makes parallel write-heavy
specs flaky.

---

## 9. Security and PDPA

- **Auth**: server-side sessions (bcrypt + a `Session` row + Gin middleware).
  **No JWT**.
- **CSRF defense**: `SameSite=Strict` cookie + Origin check. **No CSRF token**.
  `auth.SameOrigin` checks the request Origin against `AllowedOriginHost()`.
- **Login throttle**: sliding window, max 5 failures per 15 minutes per key.
  In-memory `Throttle` by default; DB-backed `DBLimiter` for multi-replica.
  `DBLimiter` **degrades safely**: on any store error it falls back to a
  per-replica in-process throttle, logs a WARN, and increments
  `login_throttle_store_error_count` (ADR-0008). It never fails open.
- **Login timing**: the login path avoids an account-enumeration timing
  side-channel (found and fixed in review).
- **Media safety**: uploads are sniffed by content type. Only JPEG, PNG, and
  WebP are accepted; the vulnerable TIFF decode path is unreachable. Images are
  downscaled and EXIF-stripped on the server.
- **PDPA / consent**: the public API, export, and media never expose phone,
  consent, or audit fields. A doctor is public only when `ConsentObtained` is
  true **and** `ReviewState = 'approved'`. Consent and review are independent
  gates. An admin delete is the PDPA-erasure exemption and overrides a year
  lock.
- **Dependency scanning**: a `vuln` CI job runs `govulncheck ./...` on every
  push/PR. The current result is 0 code-affecting vulnerabilities (the
  ClickHouse `ch-go` GO-2025-3603 advisory was cleared by a version bump).

---

## 10. Deployment

There are two supported paths. They run the same Go code.

### 10.1 Single binary (the default — ADR-0001)

Copy `server` to a host and run it. It serves the embedded UI, terminates TLS
itself via Let's Encrypt when `APP_DOMAIN` is set, and writes SQLite + media +
backups to `data/`. It can register as a systemd / Windows / launchd service.
Runbook: `docs/ops/deploy.md`. Restore: `docs/ops/restore.md`.

### 10.2 Container stack (ADR-0003 — opt-in, for scale)

`docker-compose.yaml` runs a full stack:

- **caddy** — one public origin, own ACME TLS. Proxies `/api/*` → api,
  `/media/*` → api, `/traces` → Jaeger (admin-gated by forward-auth),
  everything else → web. Load-balances across the api replicas with an
  `/api/health` active check.
- **web** — Next.js `output: standalone` on port 3000.
- **api** — the Go binary, `deploy.replicas: 2`, `APP_AUTO_MIGRATE=false`,
  behind-proxy, Postgres + Garage + DB throttle.
- **migrate** — a one-shot job that runs the DDL once (owns the admin seed
  env). The api replicas wait for it.
- **postgres** — `postgres:17-alpine`.
- **garage** — S3-compatible object storage for media (internal only,
  `dxflrs/garage:v1.0.1`).
- **garage-init** — one-shot; imports a fixed key so the api's creds match.
- **backup** — daily `pg_dump` sidecar (keeps newest 14).
- **media-backup** — rclone sidecar that pushes the Garage bucket to a single
  **off-host** remote with a dated archive (ADR-0009).
- **jaeger** — OTel trace UI at `/traces` (admin-gated).

`docker-compose.build.yaml` builds the images locally. `docker-compose.dev.yaml`
is a prod-parity dev stack (Caddy + Postgres + behind-proxy api) with hot reload
(`next dev` + `air`). Run it with `make dev`. Runbook:
`docs/ops/deploy-compose.md`.

The stack runs published images `ghcr.io/willywotz/phum-panya-{api,web}:<tag>`.

---

## 11. CI / CD (GitHub Actions)

Four workflows under `.github/workflows/`:

- **`ci.yml`** — on push (`main`, `feat/**`) and PR to `main`. Jobs: **go**
  (`go build` / `go test`), **vuln** (`govulncheck`), **web** (`tsc` + static
  export + ESLint), **e2e** (Playwright drives the real binary), **go-postgres**
  (integration test against a live Postgres), **images** (build + smoke-test the
  api and web images), **stack-validate** (`compose config` + `caddy validate`).
- **`build-images.yml`** — a reusable workflow. Builds `api` (`Dockerfile.api`)
  and `web` (`web/Dockerfile`) images and smoke-tests both. Called by `ci.yml`
  (build + smoke) and `release.yml` (also push to GHCR). Callers dictate the
  permissions.
- **`release.yml`** — on `v*` tags. A **windows** job builds `server.exe` + the
  WiX MSI and smoke-tests install/register/uninstall. A **release** job builds
  the Linux binary and publishes a GitHub Release with all three assets. It also
  pushes `ghcr.io/...:<version>` + `:latest` images (ADR-0010, CD F1).
- **`deploy.yml`** — push-based deploy (ADR-0011, CD F2, closes #19).
  `workflow_dispatch` with a `version` input (validated `N.N.N` on the runner).
  It SSHes to the VPS and runs `deploy/deploy.sh <version>`: checkout
  `v<version>` → set `APP_IMAGE_TAG` → `docker compose pull` → `up -d --wait` →
  poll `/api/health` → **auto-rollback** to the prior version on any failure.
  Rollback = dispatch an older version. Needs `DEPLOY_SSH_HOST/USER/KEY` +
  `DEPLOY_PATH` secrets. An SSH command-injection bug was caught in review and
  fixed with a runner-side version guard.

Deploy shell helpers (`deploy/deploy-lib.sh`, `deploy/backup/media-backup-lib.sh`)
keep pure logic in functions that Go tests exec via `sh`, so CI needs no docker /
rclone.

---

## 12. Architecture decision records (ADRs)

Under `docs/adr/`:

| ADR | Decision |
|---|---|
| 0001 | Single Go binary with embedded Next.js; SQLite → Postgres portable SQL; Go-native TLS |
| 0002 | Approval on-row pending model (Model B); append-only Revision |
| 0003 | Optional Postgres + Caddy compose stack (complements, not replaces, 0001) |
| 0004 | Media in Garage object storage behind the `media.Store` port |
| 0005 | Shared login throttle in a DB table (`DBLimiter`) for multi-replica |
| 0006 | Telemetry via OpenTelemetry + Jaeger (logs / metrics / traces) |
| 0007 | Migrations as a discrete release step; multi-replica behind Caddy |
| 0008 | Throttle degrades safely (never fails open) on store failure |
| 0009 | Media off-host, versioned backup (rclone remote + dated archive) |
| 0010 | Containerized release images published to GHCR (CD F1) |
| 0011 | Push-based continuous deploy with health-gate + auto-rollback (CD F2) |

---

## 13. Project history and roadmap

The full narrative is in `CONTEXT.md`. Short timeline:

- **Design phase** — data model + fill-in form designed and approved; SRS
  written; feature research and a paranoid review done. Consent/PDPA rules
  added.
- **P1 launch** — the full app, built test-first (TDD, mandatory). Released as
  **v1.0.0**.
- **P2–P5** (paid phases) — P2 approval before publish + edit history; P3 year
  locking; P4 bulk Excel import; P5 district-managed herb catalog. Frontend
  admin screens added. Released as **v1.1.0**.
- **Post-v1.1.0 hardening** — issues #12–#18: SetPhoto governance, recipe
  photos one-to-many, herb-merge guards, input validation, `full_name`,
  staff-nav role gating, web ESLint.
- **A→E compliance program** — reach full 15-Factor + Hexagonal:
  - **A** hexagonal core (ports/adapters, pure `model`).
  - **B** media → Garage object storage.
  - **C** shared DB login throttle (no per-process state left).
  - **D** telemetry (slog + OTel metrics/traces + Jaeger).
  - **E** migrations-as-release + 2 api replicas behind Caddy.
- **Hardening** — ADR-0008 (throttle degrade), ADR-0009 (off-host media
  backup), govulncheck job. Released as **v1.4.0**.
- **CD pipeline** — F1 (GHCR release images, ADR-0010) and F2 (push-based
  deploy, ADR-0011, closes #19).

### Current state (from CONTEXT.md / HANDOFF.md)

- `main` is at `cde89d2`, in sync with origin.
- 265 Go tests green; the Go suite is the source of truth.
- Latest release tag is **v1.4.0**.
- Known caveat: the `images` and `vuln` CI jobs never got a confirmed green run
  on a real runner — the confirming push hit a GitHub Actions major outage on
  2026-08-06. Re-run CI on `main`, confirm green, then cut v1.5.0.

### Testing rule

TDD is **mandatory**: write a failing test, confirm it fails, write the minimal
code to pass, confirm it passes, then refactor. Every feature branch uses
conventional names (`feat/`, `fix/`, `chore/`, `refactor/`). Multi-task work
never commits straight to `main`.

---

## 14. Where to look next

| You want… | Read |
|---|---|
| The running work log | `CONTEXT.md` |
| The operator hand-off | `docs/HANDOFF.md` |
| Build / run basics | `README.md` |
| A design decision and why | `docs/adr/00NN-*.md` |
| A feature spec | `docs/superpowers/specs/*` |
| A TDD task plan | `docs/superpowers/plans/*` |
| VPS single-binary deploy | `docs/ops/deploy.md` |
| Container stack deploy | `docs/ops/deploy-compose.md` |
| Restore from backup | `docs/ops/restore.md` |
| The domain entities | `internal/model/model.go` |
| The wiring / routes | `internal/router/router.go` |
| The config surface | `internal/config/config.go` |
