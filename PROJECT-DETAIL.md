# phum-panya — Complete Project Detail

*One self-contained document. Everything is written here. It does not point you
to other files. Built from the repository on 2026-08-09. Written in Simplified
Technical English.*

---

## Table of contents

1. What the project is
2. Product rules and roles
3. How to build, run, and test
4. Technology stack
5. Architecture (Hexagonal + 15-Factor)
6. Package-by-package map
7. Data model
8. HTTP API — every endpoint
9. Configuration — every environment variable
10. Frontend
11. Security and PDPA
12. Deployment path 1 — single binary (full runbook)
13. Deployment path 2 — container stack (full runbook)
14. Backup and restore (full steps)
15. CI/CD pipelines
16. Architecture decisions (full list, inlined)
17. Project history and current state
18. Testing and workflow rules

---

## 1. What the project is

phum-panya is a web app that keeps folk-medicine records (ตำรายาหมอพื้นบ้าน)
for one Thai province. It groups all data by district (อำเภอ).

- The **public** reads consented healer (หมอพื้นบ้าน) profiles, their recipes
  (ตำรับยา), and treatment cases (เคส).
- **Staff** write the data. There is **one central admin** and **one editor per
  district**.
- Users open the site by QR-code scan. There is **no app-store app** and **no
  map**.
- The reference product is a Thai "Tok Bidan" herbal app, but without the map.

The app ships as **one self-hosted Go binary**. The binary holds the HTTP API
(Go, Gin + GORM), the full web UI (Next.js 15, exported to static files and
embedded), and a pure-Go SQLite driver. It needs **no cgo** and **no external
database** for the default path. You copy one file to a host and run it.

The same Go code can also run as a larger container stack (PostgreSQL + Caddy +
object storage + telemetry) when an operator wants to scale.

---

## 2. Product rules and roles

- **central_admin** — sees everything, writes everything. Admin writes publish
  at once. Only an admin approves pending changes, locks a data year, runs bulk
  imports, and merges herbs.
- **district_editor** — writes only its own district's data. Every editor write
  enters a **pending** state and waits for admin approval (this is "Model B").
  An editor can never self-publish or self-approve.
- The **public** is read-only and unauthenticated. It sees a doctor, recipe, or
  case only when the record is both **consented** and **approved**.

---

## 3. How to build, run, and test

### Build

```
make build
```

`make build` runs `cd web && npm ci && npm run build`, copies the Next.js
static export from `web/out/` into `internal/webui/dist/`, then builds `./server`
with `CGO_ENABLED=0 go build -o server ./cmd/server`. You need Go and Node
installed. The result is one static, cgo-free binary that embeds the UI.

Cross-compiled release binaries:

```
make build-release
# builds bin/server-linux-amd64 and bin/server.exe (both cgo-free)
```

### Run (development)

```
APP_DEV=1 APP_ADMIN_EMAIL=admin@example.com APP_ADMIN_PASSWORD=changeme ./server
```

This serves plain HTTP on `:8080`. On first run the server seeds the first
central admin from `APP_ADMIN_EMAIL` and `APP_ADMIN_PASSWORD`, but only when no
admin exists yet.

### Run (production, single binary)

Set `APP_DOMAIN` to a real domain and do not set `APP_DEV`. The server then gets
a Let's Encrypt certificate and serves HTTPS on `:443` (with `:80` for the ACME
challenge). See section 12 for the full runbook.

### Subcommands

| Command | Action |
|---|---|
| `server` (or `server run`) | Run in the foreground, or under the OS service manager |
| `server migrate` | Run schema migration + recipe-photo backfill + admin seed once, then exit |
| `server create-admin` | Seed the first central admin, then exit |
| `server service install` | Register the OS service (Windows SCM / Linux systemd / macOS launchd) |
| `server service uninstall` | Remove the OS service |
| `server service start\|stop\|restart` | Control the OS service |

`service install` accepts `--admin-email`, `--admin-password`, `--domain`, and
`--http-addr`. It maps them to `APP_*` environment variables and bakes them into
the service definition, so the service starts already configured. Data resolves
under the service working directory: `%ProgramData%\phum-panya` on Windows, or
`/var/lib/phum-panya` elsewhere.

### Windows MSI

A release ships `phum-panya-<version>-windows-amd64.msi` (built with WiX v5,
sources in `deploy/windows/`). The wizard asks for the admin email, admin
password, and public domain, installs `server.exe` to Program Files, and
registers and starts the `phum-panya` service. Silent install:

```
msiexec /i phum-panya-<version>-windows-amd64.msi /quiet ^
  ADMINEMAIL=admin@example.com ADMINPASSWORD=changeme APPDOMAIN=example.com
```

### Test

```
go test ./...                  # Go unit + integration tests (265+ tests, 25 packages)
cd web && npx playwright test   # end-to-end browser tests (~19 specs)
```

Playwright needs `npx playwright install chromium` once. The e2e suite runs
serially (`workers: 1`), because the single shared dev SQLite database makes
parallel write-heavy specs flaky.

---

## 4. Technology stack

| Layer | Choice |
|---|---|
| Backend language | Go 1.26 (`go 1.26.0`, `toolchain go1.26.5`) |
| HTTP framework | Gin `v1.12.0` |
| ORM | GORM `v1.31.2` |
| Default database | SQLite via pure-Go `github.com/glebarez/sqlite` (cgo-free) |
| Scale database | PostgreSQL via pure-Go `gorm.io/driver/postgres` (pgx, cgo-free) |
| TLS (single binary) | Go-native ACME / Let's Encrypt (`autocert`) |
| Image processing | `disintegration/imaging` + `golang.org/x/image`; downscale ≤1600px + EXIF strip |
| Excel import/export | `xuri/excelize/v2` |
| Object storage client | `aws-sdk-go-v2` (S3 path-style, targets Garage) |
| OS service wrapper | `kardianos/service` |
| Telemetry | OpenTelemetry SDK + Prometheus exporter + `otelgin` + GORM tracing plugin |
| Password hashing | bcrypt (`golang.org/x/crypto`) |
| Frontend | Next.js 15.5 (static export or standalone), React, TypeScript |
| Styling | Tailwind CSS v4 + shadcn/ui (Radix), `next-themes` light/dark |
| Font | Sarabun (`@fontsource/sarabun`, weights 300/500/700), vendored, no CDN |
| E2E tests | Playwright |

The code is about **117 Go files** and **65 TypeScript files**, plus YAML, CSS,
and TOML. The graph holds roughly 2,492 nodes and 7,906 edges (699 functions,
197 methods, 91 routes, 89 structs, 43 interfaces).

---

## 5. Architecture (Hexagonal + 15-Factor)

The project follows **Hexagonal architecture** (ports and adapters) and the
**15-Factor App** method. These are hard project rules.

### Request flow

```
   HTTP request
        │
        ▼
   Gin engine  (internal/router — NewEngine)
   middleware: gin.Recovery, otelgin, AccessLog, RequestMetrics,
               MaxMultipartMemory = 8 MiB, auth.SameOrigin (CSRF),
               auth.LoadUser (attaches current user on /api/*)
        │  calls
        ▼
   Feature packages  (handlers)
   auth, district, user, herb, doctor, recipe, caserec, review,
   yearlock, importer, publicapi, export, backup
        │  each depends on a PORT interface, never on *gorm.DB
        ▼
   Adapters
   GORM Repo (per package) · media.Store (LocalStore | S3Store)
   auth.Sessions · auth.Limiter (Throttle | DBLimiter) · auth.Users
        │
        ▼
   internal/model  —  PURE domain entities (imports only "time")
```

### The three rules the hexagonal refactor locked in

1. `internal/model` is a **pure** domain package. It imports only `time`. The
   GORM struct tags are inert strings. `AutoMigrate` and `BackfillRecipePhotos`
   live in `internal/db`, not in `model`.
2. Every HTTP handler depends on a **port interface**, never on `*gorm.DB` or a
   concrete `*Repo`. Each feature package declares a `repository` port that its
   GORM `Repo` satisfies. The `*/port_test.go` files hold these port contracts.
   `publicapi` and `export` build their ports at the composition root. `auth`
   declares `Sessions`, `Limiter`, and `Users` ports.
3. `media.Store` is an interface. The filesystem version is `LocalStore`; the
   object-storage version is `S3Store`. `auth.Limiter` has two versions too:
   the in-process `Throttle` and the shared `DBLimiter`.

### The composition root

`internal/router/router.go` (`NewEngine`) is the composition root. It receives a
`Deps` struct (config, DB, session store, limiter, media store, clock, logger,
metrics handler, cookie-secure flag, backup parameters), builds every adapter,
and registers every module's routes.

`router` cannot live inside `httpx`: every module imports `httpx` for
`httpx.OK` / `httpx.Err`, so a back-import would make a cycle.

`cmd/server/main.go` builds `Deps` from `config.Load()`. It picks adapters by
config: `newLimiter` returns the memory `Throttle` or the DB `DBLimiter`;
`newMediaStore` returns `LocalStore` or `S3Store`. It also runs `migrateDB`
(migrate + backfill + seed admin) at startup only when `APP_AUTO_MIGRATE` is
true, and starts the daily backup ticker when the backup is enabled (SQLite
only).

### How 15-Factor is met

- **Config** — all runtime config comes from `APP_*` environment variables
  (`internal/config`). Defaults keep the single-binary path zero-config.
- **Backing services** — the database, object storage, and login throttle are
  all attached by config and swappable (SQLite/Postgres, local/S3, memory/DB).
- **Build, release, run** — the CI builds immutable container images; the
  deploy step only runs them (no build on the host).
- **Processes** — with `APP_THROTTLE_STORE=db`, sessions live in the DB, media
  in object storage, and throttle state in the DB, so **no per-process state
  remains** and the API scales to many replicas.
- **Disposability** — `httpx.ServeContext` takes a context so the supervisor can
  stop the server cleanly.
- **Logs** — `log/slog` writes JSON to stdout; the platform collects it.
- **Admin processes** — migration is a discrete `server migrate` step and a
  one-shot job in the stack.
- **Telemetry** — logs, metrics, and traces (OpenTelemetry).

---

## 6. Package-by-package map (`internal/`)

| Package | Job |
|---|---|
| `model` | Pure domain entities. No infrastructure imports. |
| `config` | Reads every `APP_*` var into `Config`; derives `UseTLS`, `CookieSecure`, `AllowedOriginHost`, `BackupEnabled`, `UsesS3Media`, `UsesDBThrottle`. |
| `db` | Opens SQLite or Postgres; `AutoMigrate` (with `UNLOGGED` login_attempts on Postgres); `BackfillRecipePhotos`; dialector helpers. |
| `httpx` | `OK`, `Err`, JSON helpers, and `ServeContext` (context-aware serve + autocert TLS). Highest fan-in helper (`Err` used ~74 times, `OK` ~47). |
| `router` | The composition root; assembles the full `gin.Engine`. |
| `clock` | `Clock` interface + `Real` clock; tests control time through it. |
| `auth` | Sessions, login/logout/current-user handlers, bcrypt password, session store, same-origin CSRF middleware, role middleware (`RequireAuth`, `RequireRole`, `LoadUser`), in-memory `Throttle`, DB `DBLimiter`. |
| `bootstrap` | `EnsureAdmin` — seeds the first central admin. A no-op when the credentials are empty (safe on a smoke-test boot). |
| `district` | District CRUD. Reads need auth; writes are admin-only. |
| `user` | Staff user CRUD, activate/deactivate, set password. Admin-only. |
| `herb` | Shared herb catalog. Editors add and edit own-district herbs; admin merges/aliases, reconciles pending, and lists. Near-duplicate lookup. |
| `doctor` | Healer profiles. Photo change, unpublish, consent gate, own-district check, review workflow, audit. |
| `recipe` | Formulas. Multi-photo child rows, resolve-doctor-by-code, year-lock guard, review workflow. |
| `caserec` | Anonymous treatment cases. Year-lock guard, review workflow. |
| `revision` | Append-only audit log. `Append` runs **after** the write transaction commits (a second pooled connection inside the tx would deadlock SQLite's WAL writer). |
| `review` | Central-admin approval queue: list queue, entry detail (current vs proposed), approve, reject with reason, approve a whole doctor tree. |
| `yearlock` | Freeze/unfreeze a `data_year`; `IsLocked` guard used by recipe/caserec. |
| `importer` | Bulk Excel import: parse the 4-sheet template → dry-run validation report → commit through the domain services → per-batch undo. |
| `publicapi` | Public read API. Filters `consent_obtained = true` AND `review_state = 'approved'`; hides phone, consent, audit, and pending fields. |
| `export` | Staff CSV / XLSX export. |
| `backup` | Nightly single-zip backup (DB + media), retention, and the `POST /api/backup/run` route. SQLite path only. |
| `media` | `Store` port; `LocalStore` (filesystem) and `S3Store` (Garage); image downscale + EXIF strip; content-type sniff that accepts only JPEG/PNG/WebP; storage-usage meter. |
| `telemetry` | slog JSON logger, OTel metrics (`/metrics`), OTel traces, `AccessLog` middleware, `RequestMetrics` middleware, `InstrumentDB`, `Setup`. |
| `svc` | `kardianos/service` wrapper: `Run` (under the supervisor) and `Control` (install/uninstall/start/stop/restart). |
| `webui` | Embeds `dist/` with `embed.FS`; serves the SPA and assets; returns a JSON 404 for unknown `/api/*` paths so the SPA catch-all never shadows the API. |

---

## 7. Data model

Defined in `internal/model/model.go`. Relationships:

```
District ──< Doctor ──< Recipe ──< Case
                          │
                          ├──< RecipePhoto
                          └──< Ingredient >── Herb (one shared catalog)

Revision   (append-only): entity_type + entity_id → who / when / action / after_json
YearLock   (data_year PK): freezes that year's Recipe/Case, read-only
ImportBatch (id PK): imported_by/at, source_file, row_count, status; Doctor/Recipe/Case carry BatchID
LoginAttempt (id PK): shared login-throttle rows (UNLOGGED on Postgres)
Session    (token_hash PK): server-side login session
```

### Entities

- **District** (อำเภอ) — the grouping unit. `Name`, `Province`. A fixed list; a
  delete never cascades onto its doctors.
- **User** — a staff login. `FullName`, `Email` (unique), `PasswordHash` (never
  serialized to JSON), `Role` (`central_admin` | `district_editor`, check
  constraint), `DistrictID` (nullable), `Active`.
- **Session** — `TokenHash` (primary key), `UserID`, `ExpiresAt`. Session TTL is
  7 days.
- **Doctor** (หมอพื้นบ้าน) — the healer. One row per healer with `FirstYear`.
  Fields include `Code` (unique, for paper-form linking), `Photo`, `FullName`,
  `KnownAs`, `Gender`, `BirthYear`, `DistrictID`, `Address`, `Phone`,
  `Specialty`, `YearsExperience`, `Lineage`, `Status`
  (active/inactive/deceased), and the consent + governance columns below.
- **Herb** (สมุนไพร) — ONE shared province-wide catalog. `ThaiName`,
  `LocalName`, `ScientificName`, `Photo`, `PartUsed`, `Properties`. P5 added
  `CreatedByDistrictID` (provenance) and `AliasOfID` (set = this herb is an
  alias of a canonical herb).
- **Recipe** (ตำรับยา) — a formula owned by one doctor. `Code` (unique), `Name`,
  `DoctorID`, `Indication`, `Preparation`, `Usage`, `Caution`, `CareStage`,
  `DataYear`, a `Photos []RecipePhoto` child list, and governance columns.
- **RecipePhoto** — one image on a recipe. `RecipeID` (FK, ON DELETE CASCADE),
  `Path`, `SortOrder`. A recipe can hold many. This replaced the old single
  `Recipe.Photo` string.
- **Ingredient** — one row inside a recipe. Exactly one of `HerbID` or
  `PendingHerbName` is set; a `chk_herb_xor` check enforces this. The
  pending-herb name lets district entry continue when a herb is not in the
  catalog yet. Also `Amount`, `Unit`, `Note`.
- **Case** (เคส) — an anonymous treatment result linked to one recipe. Patient
  is anonymous: `PatientGender`, `PatientAgeRange` only. Also `Condition`,
  `Treatment`, `Result` (cured/better/no_change), `Duration`, `Photo`,
  `DataYear`, and governance columns.
- **Revision** — append-only audit entry. `EntityType`, `EntityID`,
  `ChangedBy`, `ChangedAt`, `Action` (create/update/delete/reject), `AfterJSON`.
- **YearLock** — `DataYear` (primary key), `LockedAt`, `LockedBy`.
- **ImportBatch** — `ImportedBy`, `ImportedAt`, `SourceFile`, `RowCount`,
  `Status` (committed/undone).
- **LoginAttempt** — `Key`, `CreatedAt`. The DB limiter reads these rows so
  throttle state is shared across replicas.

### Governance columns (Doctor, Recipe, Case share these)

- `ReviewState` — `approved` | `pending` | `rejected` (check constraint;
  default `approved` so existing rows stay public).
- `PendingJSON` — the staged edit overlay (nullable).
- `PendingPhoto` — a staged photo change (Doctor and Case).
- `PendingDelete` — a queued delete.
- `RejectionReason` — why an admin rejected the change.
- `UpdatedBy`, `UpdatedAt`, `BatchID`.

### Migration behaviour

GORM `AutoMigrate` applies new columns and tables on startup (or in the
`migrate` job). New governance columns default to `approved`, so existing rows
stay public. Enum `check:` constraints apply on a **fresh** migrate only —
SQLite cannot `ALTER`-add a CHECK, so an already-deployed SQLite database relies
on the request-binding validation instead. No manual migration step is needed;
schema changes here are additive.

---

## 8. HTTP API — every endpoint

All routes use **full-English names**, never short aliases (a project rule):
for example `/api/current-user`, not `/api/me`. Every `/api/*` route carries
`auth.LoadUser`, so the current user is attached when a session cookie is sent.

### Public (no authentication)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | Liveness check; returns `{"status":"ok"}` |
| GET | `/api/public/districts` | List districts |
| GET | `/api/public/doctors` | List consented, approved healers (`?district_id=`, `?q=`) |
| GET | `/api/public/doctors/:id` | One healer detail (with nested approved recipes/cases) |
| GET | `/api/public/herbs` | List herbs (aliases filtered out) |
| GET | `/api/public/recipes` | List recipes (`?district_id=`, `?herb_id=`, `?q=`) |
| GET | `/media/*key` | Stream an uploaded image through the media adapter; immutable cache header; missing key → 404 |

### Authentication

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/login` | Log in (bcrypt verify + throttle) |
| POST | `/api/logout` | Log out |
| GET | `/api/current-user` | Current user (includes `full_name`) |
| GET | `/api/authorization/verify-admin` | Caddy forward-auth target that gates `/traces`; central-admin → 204, else 401 |

### Staff records (require auth; a district editor is scoped to its own district)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/districts` | List districts (any staff) |
| POST / PUT / DELETE | `/api/districts`, `/api/districts/:id` | District writes (admin-only) |
| GET / POST | `/api/users`, PUT `/api/users/:id` | Staff user list, create, update (admin-only) |
| POST | `/api/users/:id/active` | Activate/deactivate a user (admin-only) |
| POST | `/api/users/:id/password` | Set a user's password (admin-only) |
| GET | `/api/herbs` | List herbs (admin) |
| POST | `/api/herbs` | Create a herb (any staff; stamped with the editor's district) |
| PUT | `/api/herbs/:id` | Update a herb (editor may edit only own-district herbs → 403 otherwise) |
| DELETE | `/api/herbs/:id` | Delete a herb (admin) |
| GET | `/api/herbs/near-duplicates?thaiName=` | Save-time near-duplicate nudge |
| GET | `/api/herbs/pending` | List pending-herb names (admin) |
| POST | `/api/herbs/reconcile` | Resolve a pending herb into the catalog (admin) |
| POST | `/api/herbs/:id/merge/:canonicalId` | Mark a herb an alias of a canonical one and re-point ingredients (admin) |
| GET | `/api/storage` | Media storage-usage meter (admin) |
| GET / POST | `/api/doctors`, PUT/DELETE `/api/doctors/:id` | Doctor CRUD |
| POST | `/api/doctors/:id/photo` | Change a doctor's photo |
| POST | `/api/doctors/:id/unpublish` | Unpublish a doctor |
| GET / POST | `/api/recipes`, PUT/DELETE `/api/recipes/:id` | Recipe CRUD |
| POST | `/api/recipes/:id/photo` | Append a recipe photo |
| GET | `/api/recipes/resolve-doctor` | Resolve a doctor by code |
| GET / POST | `/api/cases`, PUT/DELETE `/api/cases/:id` | Case CRUD |
| POST | `/api/cases/:id/photo` | Change a case photo |
| GET | `/api/export/doctors.csv`, `/api/export/doctors.xlsx` | Bulk export (and the same pattern for other entities) |

A district-editor create/update/delete on Doctor/Recipe/Case enters the pending
queue (HTTP behaviour unchanged, but the row becomes `pending`). A central-admin
write is immediate and logged.

### Governance (central-admin only)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/review/queue` | Pending approval queue |
| GET | `/api/review/entry/:entityType/:entityId` | Current-vs-proposed detail (`entityType` = doctor/recipe/case) |
| POST | `/api/review/entry/:entityType/:entityId/approve` | Approve one change |
| POST | `/api/review/entry/:entityType/:entityId/reject` | Reject (body: `{"reason":"..."}`) |
| POST | `/api/review/doctor/:doctorId/approve-all` | Approve a whole doctor tree |
| GET | `/api/year-locks` | List locked years |
| POST | `/api/year-locks` | Lock a year (body: `{"dataYear":2567}`) |
| DELETE | `/api/year-locks/:dataYear` | Unlock a year |
| POST | `/api/imports?dryRun=true\|false` | Upload an `.xlsx` (field `file`); dry-run report or commit |
| POST | `/api/imports/:batchId/undo` | Undo a bulk-import batch |
| POST | `/api/backup/run` | Run a backup now |

Year-lock rules: a year locks only when its pending Recipe/Case queue is empty.
Once locked, recipe/case create and update are refused for **all** writers
(HTTP 409); an editor's queued delete is refused; an **admin delete is the
PDPA-erasure exemption and is always allowed**. The approve path re-checks the
target year, so a cross-year pending edit cannot be promoted into a year that
was locked after the edit was queued.

### Internal (not routed publicly by Caddy)

| Method | Path | Purpose |
|---|---|---|
| GET | `/metrics` | Prometheus metrics (`http_server_request_count`, `_duration`, Go runtime) |

---

## 9. Configuration — every environment variable

All config comes from `APP_*` variables. Defaults keep the single-binary path
zero-config.

| Variable | Default | Purpose |
|---|---|---|
| `APP_HTTP_ADDR` | `:8080` | Listen address |
| `APP_DOMAIN` | unset | Public domain; enables built-in TLS when set (and not dev, not behind proxy) |
| `APP_DEV` | unset | `1` = plain HTTP, no TLS |
| `APP_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `APP_DB_PATH` | `data/app.db` | SQLite file path |
| `APP_DATABASE_URL` | unset | Postgres DSN |
| `APP_AUTO_MIGRATE` | `true` | Migrate at startup; the stack sets `false` and runs a one-shot `migrate` job |
| `APP_BEHIND_PROXY` | unset | `1` = trust a reverse proxy (Caddy) |
| `APP_PUBLIC_ORIGIN` | unset | Public origin URL; drives the CSRF host and the cookie Secure flag |
| `APP_MEDIA_DIR` | `data/media` | Local media directory |
| `APP_MEDIA_DRIVER` | `local` | `local` or `s3` (Garage) |
| `APP_S3_ENDPOINT` | unset | Object-storage endpoint (e.g. `http://garage:3900`) |
| `APP_S3_REGION` | `garage` | S3 region |
| `APP_S3_BUCKET` | `media` | Bucket name |
| `APP_S3_ACCESS_KEY` | unset | Access key — Garage format `GK` + 24 hex |
| `APP_S3_SECRET_KEY` | unset | Secret key — 64 hex |
| `APP_S3_USE_PATH_STYLE` | `true` | Path-style S3 addressing |
| `APP_THROTTLE_STORE` | `memory` | `memory` (per-process) or `db` (shared across replicas) |
| `APP_BACKUP_DIR` | `data/backup` | Nightly zip output (SQLite path only) |
| `APP_ADMIN_EMAIL` | unset | First central admin email (seeded once) |
| `APP_ADMIN_PASSWORD` | unset | First central admin password (seeded once) |
| `APP_LOG_LEVEL` | `info` | slog level |
| `APP_LOG_FORMAT` | `json` | slog format |
| `APP_OTLP_ENDPOINT` | unset | OTLP gRPC trace target; degrades gracefully when unset (spans still made, `trace_id` still logged) |
| `APP_SERVICE_NAME` | `phum-panya-api` | Telemetry service name |

Derived rules (`internal/config/config.go`):

- `UseTLS()` — true only when not dev, not behind proxy, and a domain is set.
- `CookieSecure()` — true under production TLS; behind a proxy it is true only
  when the public origin is HTTPS. So a dev stack over plain HTTP still gets a
  usable session cookie.
- `AllowedOriginHost()` — the CSRF check host: the public-origin host behind a
  proxy, otherwise the domain.
- `BackupEnabled()` — false on Postgres (the `pg_dump` sidecar owns backups).

---

## 10. Frontend

Next.js 15 App Router, TypeScript, exported to static HTML for the single-binary
path (no Node server there). Two route groups under `web/app/`:

- `(public)` — landing page, `doctors`, `doctor`, `herbs`, `recipes`. Public
  read-only browsing.
- `(staff)` — `login`, then `staff/*`: `cases`, `districts`, `doctors`,
  `herbs`, `imports`, `recipes`, `review`, `users`, `year-locks`.

Shared pieces:

- **UI kit** — shadcn/ui (Radix) under `web/components/ui/*`. Shared building
  blocks: `CrudForm`, `CrudTable`, `IngredientEditor`, `PhotoUpload`,
  `HerbAddForm`, `ExportLinks`. Every single-select is a Radix `Select`; the
  add/edit form is a modal `Dialog`; delete is an `AlertDialog`. Only the doctor
  `specialty` multi-select stays a native `<select multiple>` (Radix has no
  multi-select).
- **Theme** — a warm herbal theme with a light/dark toggle (`next-themes`).
- **Font** — Sarabun, vendored (Thai subset bundled, no CDN). Body weight 300,
  titles weight 500.
- **i18n** (`web/lib/i18n.tsx`) — bilingual: Thai default, English label toggle.
  The content itself is not translated.
- **API client** (`web/lib/api.ts`) — relative `fetch('/api/...')`, so
  same-origin proxying works with no code change. `api.upload` handles multipart.
- **Auth / role gating** (`web/lib/auth.tsx`) — the staff nav shows the
  admin-only links (`review`, `year-locks`, `imports`, `districts`, `users`)
  only to a central admin; a `RequireAdmin` guard bounces a district editor off
  an admin URL.
- **Print** — a `print.css` rule hides the nav (`.no-print`) for the public
  print/PDF page.
- **Dev build** — Turbopack (`next dev --turbopack`) for fast HMR. The
  production build (`export` or `standalone`) stays webpack.

Staff screens exist for every backend flow: the approval queue
(`/staff/review`), year locks (`/staff/year-locks`), bulk import
(`/staff/imports` — upload + dry-run report + commit + undo), and herb
merge/near-duplicate (`/staff/herbs`).

---

## 11. Security and PDPA

- **Authentication** — server-side sessions (bcrypt + a `Session` row + Gin
  middleware). **No JWT**. Revocation is instant (delete the row).
- **CSRF defense** — a `SameSite=Strict` session cookie plus an Origin check
  (`auth.SameOrigin` matches the request Origin against `AllowedOriginHost()`).
  **No CSRF token**.
- **Login throttle** — a sliding window, max 5 failures per 15 minutes per key.
  The in-memory `Throttle` is the default; the DB-backed `DBLimiter` shares
  state across replicas. `DBLimiter` **degrades safely**: on any store error it
  falls back to a per-replica in-process throttle, logs a WARN, and increments
  the counter `login_throttle_store_error_count`. It never fails open.
- **Login timing** — the login path avoids an account-enumeration timing
  side-channel (found and fixed in review).
- **Media safety** — uploads are sniffed by content type; only JPEG, PNG, and
  WebP are accepted, which makes the vulnerable TIFF decode path unreachable.
  Images are downscaled (≤1600px) and EXIF-stripped on the server.
- **PDPA / consent** — the public API, export, and media never expose phone,
  consent, audit, `pending_json`, `rejection_reason`, or `Revision.after_json`.
  A doctor/recipe/case is public only when `consent_obtained = true` **and**
  `review_state = 'approved'` — two independent gates, applied in one place
  (`publicapi`). An admin delete is the PDPA-erasure exemption and overrides a
  year lock. Herb provenance is stamped server-side and is immutable through an
  update, so ownership cannot be hijacked.
- **Own-district enforcement** — every staff write checks the editor's district,
  including a PUT's dual old-and-new district check.
- **Dependency scanning** — a `vuln` CI job runs `govulncheck ./...` on every
  push and PR. The current result is 0 code-affecting vulnerabilities. The
  ClickHouse `ch-go` advisory GO-2025-3603 (pulled transitively by the GORM OTel
  plugin) was cleared by bumping `clickhouse-go/v2` to v2.48.0
  (`ch-go v0.74.0`), and `toolchain go1.26.5` patches 19 standard-library CVEs.

---

## 12. Deployment path 1 — single binary (full runbook)

This is the **default** path. The server is one static, cgo-free binary that
embeds the UI and stores all state under `data/`. It terminates TLS itself, so
no nginx or Caddy is needed.

### Before you start

- A VPS reachable over SSH, with root/sudo.
- A domain whose DNS **A record points at the VPS public IP**.
- Ports **80 and 443** open to the internet (Let's Encrypt validates over 80;
  the app serves over 443). Open them in the cloud firewall and the host
  firewall (`ufw allow 80,443/tcp`).
- A long, unique admin password ready.

### Steps

1. **Put the binary on the host:**

   ```bash
   curl -fL -o /usr/local/bin/phum-panya \
     https://github.com/willywotz/phum-panya/releases/download/<tag>/server-<tag>-linux-amd64
   chmod +x /usr/local/bin/phum-panya
   /usr/local/bin/phum-panya --help 2>&1 | head -1
   ```

2. **Install and start the service.** The binary registers itself with systemd
   and bakes the seed config into the unit:

   ```bash
   phum-panya service install \
     --admin-email=admin@your-domain.org \
     --admin-password='<LONG-RANDOM-PASSWORD>' \
     --domain=your-domain.org
   phum-panya service start
   ```

   This creates the systemd unit `phum-panya` (data under
   `/var/lib/phum-panya`). On first start the DB is migrated and the first admin
   is seeded. Because `--domain` is set (and `APP_DEV` is not), the server gets a
   Let's Encrypt certificate and serves HTTPS on `:443`.

   Data layout under `/var/lib/phum-panya`:

   | Path | What |
   |---|---|
   | `data/app.db` | SQLite database (`APP_DB_PATH`) |
   | `data/media/` | uploaded photos (`APP_MEDIA_DIR`) |
   | `data/backup/` | nightly backup zips, newest 14 kept (`APP_BACKUP_DIR`) |

3. **Verify it is up:**

   ```bash
   systemctl status phum-panya --no-pager
   journalctl -u phum-panya -n 50 --no-pager      # watch first-boot + ACME logs
   curl -fsS https://your-domain.org/api/health   # expect {"status":"ok"}
   ```

   Then open the site, sign in at `/login` with the seeded admin, and confirm
   the staff nav shows the admin screens.

4. **Run the live acceptance walk:** as admin, create a district and an editor
   user; as the editor, create a doctor/recipe/case and confirm it enters the
   pending queue (not the public site); as admin, approve it in `/staff/review`
   and confirm it appears publicly (only after consent + approval).

### Upgrade

```bash
phum-panya service stop
curl -fL -o /usr/local/bin/phum-panya \
  https://github.com/willywotz/phum-panya/releases/download/<new-tag>/server-<new-tag>-linux-amd64
chmod +x /usr/local/bin/phum-panya
phum-panya service start      # AutoMigrate applies new columns/tables on boot
```

Data in `/var/lib/phum-panya` is untouched by a binary swap. Take a backup
before upgrading a production host.

### Rollback

Re-download the previous tag's binary over `/usr/local/bin/phum-panya` and
`service restart`. Migrations here are additive, so a straight binary rollback
is normally safe. If a bad migration corrupted data, restore the latest good
backup zip (section 14).

---

## 13. Deployment path 2 — container stack (full runbook)

This is an **optional** path for operators who want containers and horizontal
scale. It runs the same Go code. `docker-compose.yaml` defines the stack.

### Services

- **caddy** (`caddy:2-alpine`) — one public origin with its own ACME TLS on
  ports 80/443. Proxies `/api/*` and `/media/*` to the api, `/traces` to Jaeger
  (admin-gated by a forward-auth call to `/api/authorization/verify-admin`), and
  everything else to web. Load-balances across the api replicas with an
  `/api/health` active check.
- **web** (`ghcr.io/willywotz/phum-panya-web:<tag>`) — Next.js `standalone` on
  port 3000.
- **api** (`ghcr.io/willywotz/phum-panya-api:<tag>`) — the Go binary,
  `deploy.replicas: 2`, `APP_AUTO_MIGRATE=false`, behind-proxy, Postgres +
  Garage + DB throttle + OTLP to Jaeger.
- **migrate** — a one-shot job (`command: ["migrate"]`) that runs the DDL once
  and owns the admin-seed env. The api replicas wait for it to complete.
- **postgres** (`postgres:17-alpine`) — the database, with a health check.
- **garage** (`dxflrs/garage:v1.0.1`) — S3-compatible object storage for media,
  internal only (no published port).
- **garage-init** — a one-shot that creates the bucket and imports a fixed key
  so the api's credentials match. It shares Garage's network namespace.
- **backup** (`postgres:17-alpine`) — a daily `pg_dump` sidecar, keeps the
  newest 14 dumps in a volume.
- **media-backup** (`rclone/rclone:latest`) — pushes the Garage bucket to a
  single **off-host** rclone remote, with dated `--backup-dir` archives and
  retention (`MEDIA_BACKUP_KEEP_DAYS`, default 30). The remote backend is chosen
  by the operator via `RCLONE_CONFIG_DEST_*` (s3 or sftp).
- **jaeger** (`jaegertracing/all-in-one:1.62.0`) — the OTel trace UI, served at
  `/traces` (admin-gated), OTLP enabled.

### Run it

1. Copy `.env.example` to `.env` and set the secrets. The Garage key formats are
   enforced:
   - `APP_S3_ACCESS_KEY` = `GK` + 24 hex (`echo "GK$(openssl rand -hex 12)"`)
   - `APP_S3_SECRET_KEY` and `GARAGE_RPC_SECRET` = 64 hex (`openssl rand -hex 32`)
   - plus `APP_DOMAIN`, `POSTGRES_PASSWORD`, `APP_ADMIN_PASSWORD`.
2. `docker compose up -d`. The `depends_on` conditions order the boot: garage →
   garage-init → migrate → 2×api → caddy. The stack runs published images
   `ghcr.io/willywotz/phum-panya-{api,web}:${APP_IMAGE_TAG:-latest}`. To build
   images locally instead of pulling, use `docker-compose.build.yaml`.
3. Observability: `/metrics` is internal-only (scrape `api:8080/metrics`);
   traces are at `https://<domain>/traces` (central-admin login required); logs
   are JSON on stdout with a `trace_id`.

### Dev stack (prod parity, hot reload)

`docker-compose.dev.yaml` mirrors prod: Caddy (reusing the prod Caddyfile),
Postgres, and the api in behind-proxy mode, so the real Origin/CSRF and Postgres
code paths run in dev. The only divergence is that web and api stay hot-reload
(`next dev` + `air`). Run it with `make dev`
(`docker compose -f docker-compose.dev.yaml up -w --build --force-recreate`).

---

## 14. Backup and restore (full steps)

### What is backed up

- **Single binary (SQLite)** — the server writes a dated zip
  (`backup-<date>.zip`, holding `app.db` + the `media/` folder) to
  `data/backup/` every 24 hours and keeps the newest 14. You can also trigger
  one with `POST /api/backup/run`.
- **Container stack (Postgres)** — the `backup` sidecar runs a daily `pg_dump`
  and keeps the newest 14. Media durability comes from the `media-backup`
  off-host rclone push.

**Off-site copies are the owner's job.** Schedule a pull of the backup zips (or
the pg_dump output) to another host or object storage.

### Restore (single binary / SQLite)

1. Stop the `phum-panya` service.
2. Choose the backup zip, for example `backup-2026-08-01.zip`.
3. Unzip it: `unzip backup-2026-08-01.zip -d /tmp/restore`.
4. Copy `app.db` from the unzipped folder to `APP_DB_PATH`, replacing the file.
5. Copy the unzipped `media/` contents to `APP_MEDIA_DIR`, replacing files.
6. Start the `phum-panya` service.
7. Confirm the app loads and recent data looks correct.

### Restore (container stack / media)

Restore media from the off-host remote with
`rclone copy dest:$PATH/current garage:$BUCKET` (or `.../archive/<stamp>` for a
point in time). Restore Postgres from the newest `pg_dump` file.

---

## 15. CI/CD pipelines

Four GitHub Actions workflows under `.github/workflows/`:

- **`ci.yml`** — on push (`main`, `feat/**`) and PR to `main`. Jobs: **go**
  (`go build` / `go test`), **vuln** (`govulncheck`), **web** (`tsc` + static
  export + ESLint), **e2e** (Playwright drives the real binary), **go-postgres**
  (integration test against a live Postgres), **images** (build + smoke-test the
  api and web images), **stack-validate** (`docker compose config` +
  `caddy validate`).
- **`build-images.yml`** — a reusable workflow. Builds the api
  (`Dockerfile.api`) and web (`web/Dockerfile`) images and smoke-tests both (api
  via its `HEALTHCHECK`, web via `curl :3000`). `ci.yml` calls it for
  build+smoke; `release.yml` calls it to also push to GHCR. The caller dictates
  the permissions (a hard-declared `packages: write` in the reusable workflow
  caused a `startup_failure`, so it was removed).
- **`release.yml`** — on `v*` tags. A **windows** job builds `server.exe` + the
  WiX MSI and smoke-tests silent install/register/uninstall. A **release** job
  builds the Linux binary and publishes a GitHub Release with all three assets
  (Linux binary, Windows exe, MSI). It also pushes
  `ghcr.io/willywotz/phum-panya-{api,web}:<version>` and `:latest`. Tag a commit
  that already passed `ci`.
- **`deploy.yml`** — push-based deploy (`workflow_dispatch` with a `version`
  input, validated `N.N.N` on the runner). It SSHes to the VPS and runs
  `deploy/deploy.sh <version>`: checkout `v<version>` → set `APP_IMAGE_TAG` →
  `docker compose pull` → `up -d --wait` → poll `/api/health` →
  **auto-rollback** to the prior version and fail the run on any failure.
  Rollback is a dispatch of an older version. It needs the repo secrets
  `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`, `DEPLOY_SSH_KEY`, and `DEPLOY_PATH`, plus
  host SSH. An SSH command-injection bug was caught in review and fixed with a
  runner-side version guard.

Deploy shell logic (`deploy/deploy-lib.sh`,
`deploy/backup/media-backup-lib.sh`) keeps pure functions
(`validate_version`, `read_tag`, `write_tag`, `archive_path`, `prune_plan`,
`require_env`) that Go tests exec through `sh`, so CI needs no docker or rclone.

Before the first real deploy dispatch, the owner must set the Actions secrets,
clone the repo at `DEPLOY_PATH` on the host, populate `.env` (including
`APP_IMAGE_TAG`), and `docker login ghcr.io` if the packages are private.

---

## 16. Architecture decisions (full list, inlined)

- **0001 — Single Go binary with embedded Next.js.** One cgo-free binary embeds
  the Next.js static export and uses SQLite by default. The SQL stays portable
  so Postgres is possible. TLS is Go-native ACME (the client provides the
  domain). This is the default and it stays the default.
- **0002 — Approval on-row pending model (Model B).** An editor save does not
  publish; it stages a pending change on the row (`pending_json` /
  `pending_delete` / `pending_photo` + `review_state`). An admin approves it.
  Each approved change writes one append-only `Revision`. Consent and review are
  independent public gates. `Revision.Append` runs after the write tx commits to
  avoid a SQLite WAL deadlock.
- **0003 — Optional Postgres + Caddy compose stack.** A second, opt-in deploy
  path. It complements ADR-0001; it does not replace it. All new behaviour is
  gated behind config, so the single-binary default is unchanged.
- **0004 — Media in Garage object storage.** Uploaded images can live in Garage
  (S3-compatible) behind the `media.Store` port, selected by
  `APP_MEDIA_DRIVER`. The app streams `GET /media/<key>` through the port.
- **0005 — Shared login throttle in the DB.** The login throttle moved to a
  shared `login_attempts` table (`auth.DBLimiter`), selected by
  `APP_THROTTLE_STORE`. This removed the last per-process state. On Postgres the
  table is `UNLOGGED`. SQLite stays.
- **0006 — Telemetry via OpenTelemetry + Jaeger.** slog JSON logs, OTel metrics
  at `/metrics` (Prometheus exporter), and OTel traces to Jaeger. A `trace_id`
  correlates logs and traces. The Jaeger UI at `/traces` is admin-gated.
- **0007 — Migrations as a release step + multi-replica.** A `server migrate`
  subcommand and the `APP_AUTO_MIGRATE` gate. Prod runs a one-shot migrate job
  and two api replicas behind Caddy load-balancing.
- **0008 — Throttle degrades safely.** `DBLimiter` no longer fails open when
  Postgres is down. It degrades to a per-replica in-process throttle, logs a
  WARN, and increments `login_throttle_store_error_count`. Outage-time failures
  are not written back on recovery (accepted trade-off).
- **0009 — Off-host, versioned media backup.** The `media-backup` sidecar pushes
  the Garage bucket to a single off-host rclone remote with dated archives and
  retention, instead of a same-host volume. This fixes the "lose the disk, lose
  the backup" gap.
- **0010 — Containerized release images.** CI builds immutable api/web images and
  publishes them to GHCR on a `v*` tag. Compose runs the published image tag.
  This fixes a 15-Factor build/release/run violation (the stack used to build
  from source on the deploy host).
- **0011 — Push-based continuous deploy.** The `deploy.yml` workflow SSHes to the
  host, pulls the target image tag, brings the stack up with a health gate, and
  auto-rolls-back on failure.

---

## 17. Project history and current state

Short timeline:

- **Design phase** — the data model and the standard fill-in form were designed
  and approved; a software requirements spec (SRS) was written; feature research
  and a paranoid fresh-context review were done. Consent/PDPA rules were added
  (consent date, opt-out, a "must-do before launch" list). Record linking uses a
  short `code` on Doctor and Recipe. One Doctor row per healer with `first_year`;
  Recipe/Case carry `data_year`. A pending-herb fallback keeps district entry
  from being blocked.
- **P1 launch** — the full app, built test-first (TDD). Released as **v1.0.0**.
  Reviews caught and fixed real defects: a login timing side-channel, a case PUT
  cross-district re-parent bypass, a backup file-descriptor leak, and photo
  blanking on edit.
- **P2–P5 (paid phases)** — P2 approval before publish + edit history; P3 year
  locking; P4 bulk Excel import; P5 district-managed herb catalog. Frontend
  admin screens and role-gated navigation were added. Released as **v1.1.0**.
- **Post-v1.1.0 hardening (issues #12–#18, #27, #28)** — SetPhoto governance,
  recipe photos one-to-many, herb-merge guards, input validation (`binding` +
  enum CHECKs), `full_name` on current-user, staff-nav role gating, web ESLint.
  Two HIGH defects were caught in review and fixed: a cross-year pending edit
  that could be approved into a later-locked year, and a bulk-undo that could
  cascade-delete other batches' rows.
- **A→E compliance program** — brought the project to full 15-Factor +
  Hexagonal compliance in five sub-projects: A hexagonal core (ports/adapters,
  pure `model`), B media → Garage, C shared DB login throttle, D telemetry
  (slog + OTel + Jaeger), E migrations-as-release + two api replicas behind
  Caddy. Net effect: no per-process state; the api scales horizontally. Verified
  live (two replicas round-robined, migrate ran once, Caddy failed over when a
  replica was killed).
- **Hardening batch** — ADR-0008 (throttle degrade), ADR-0009 (off-host media
  backup), and a `vuln` (govulncheck) CI job that surfaced and fixed
  GO-2025-3603. Released as **v1.4.0**.
- **CD pipeline** — F1 (GHCR release images, ADR-0010) and F2 (push-based
  deploy with health-gate and auto-rollback, ADR-0011). This closed the "no CD"
  issue (#19).

### Current state

- `main` is in sync with the remote; the latest release tag is **v1.4.0**.
- **265 Go tests pass** locally; the Go suite is the source of truth.
- `govulncheck` reports 0 code-affecting vulnerabilities; 0 open Dependabot
  alerts.
- **One caveat:** the `images` and `vuln` CI jobs never got a confirmed green
  run on a real runner — the confirming push hit a GitHub Actions major outage
  on 2026-08-06. The next step is to re-run CI on `main`, confirm both jobs are
  green, and then cut v1.5.0.
- **Deferred (non-blocking):** zero-downtime rolling replica restart (the deploy
  accepts brief downtime), multi-host Garage redundancy (single node + off-host
  backup is accepted), per-worker e2e DB isolation (the suite runs serially),
  and the unformatted recipe-payload cell in the review-queue diff.

---

## 18. Testing and workflow rules

- **TDD is mandatory.** Write a failing test, confirm it fails, write the
  minimal code to pass, confirm it passes, then refactor.
- **Branch first.** Multi-step work uses a new branch with a conventional name
  (`feat/`, `fix/`, `chore/`, `refactor/`). Multi-task work never commits
  straight to `main`.
- **API names are full English**, never short aliases.
- **Code style** follows the language style guides (Uber Go, Google
  HTML/CSS/JS/TS/Python). Prefer clean, minimal code; American English names;
  avoid `xxxList` plurals; sort imports by path; comment only the non-obvious.
- **Compliance** with the 15-Factor App method and Hexagonal architecture is
  required, not optional.
