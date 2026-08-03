# phum-panya — Handoff

Date: 2026-08-04 · Branch: **`main`** · Released: **`v1.0.0`**.
Everything below is merged to `main` (107 commits) and shipped as the tagged `v1.0.0`
release. There is one trunk — all feature branches are merged and deleted.

## 1. What this is

A web app to collect **folk-medicine records** (ตำรายาหมอพื้นบ้าน) for one province,
grouped by district (อำเภอ). The public reads everything; one editor per district plus a
central admin write the data. Access is by QR → browser. No app-store app, no map.

It ships as **one self-hosted Go binary** with the Next.js UI embedded and data in SQLite.

- Client context: `messages.txt`. Glossary/status: `CONTEXT.md`.
- Spec: `docs/superpowers/specs/2026-08-03-srs.md` (SRS, FR-*/NFR-*).
- Data model: `docs/superpowers/specs/2026-08-03-data-model-and-form-design.md`.
- Stack decision: `docs/adr/0001-single-go-binary-embedded-nextjs.md`.
- Implementation plan (33 tasks): `docs/superpowers/plans/2026-08-03-p1-launch.md`.
- Ops: `README.md`, `docs/ops/restore.md`.

## 2. Status

**P1 is functionally complete, green, merged to `main`, and released as `v1.0.0`.** Built
task-by-task with TDD + fresh-context review per task. Five bodies of work have landed:

- **P1 launch** — staff CRUD (Doctor/Recipe/Case), users/districts, herb catalog + pending-herb
  reconcile, public read/search/filter, consent + PDPA, staff bulk export, nightly backup + restore.
- **Styling pass** — Tailwind v4 + shadcn/ui (Radix), warm herbal theme + light/dark toggle, a real
  landing page, and a vendored Thai font. All offline; the single embedded binary is unchanged.
- **Dev compose stack** — frontend and API as separate containers behind nginx for hot-reload
  development (`make dev`); prod stays the single binary.
- **Go 1.26 + Sarabun** — toolchain bumped to Go 1.26; the vendored font is now **Sarabun**
  (body weight 300, titles 500).
- **Windows service + MSI + CI/CD** — the binary runs as an OS service (kardianos), a WiX MSI
  installs it on Windows, and GitHub Actions gates every push and cuts releases on tags.

- **114 Go tests** (21 packages) + **9 Playwright e2e specs / 13 tests** (incl. the full SRS
  §6.1 UAT, plus theme-toggle and landing specs).
- cgo-free single binary via `make build`; the prod Docker image builds and runs (health,
  embedded styled UI, vendored font, seeded-admin login verified in-container).
- **`v1.0.0` released** with three assets: `server-v1.0.0-linux-amd64`,
  `server-v1.0.0-windows-amd64.exe`, `phum-panya-v1.0.0-windows-amd64.msi`.

## 3. Stack (as built)

| Layer | Choice |
|---|---|
| Runtime | One **Go** binary, `CGO_ENABLED=0` static, **`go 1.26`** |
| HTTP | **Gin** |
| DB | **SQLite** via **GORM** + pure-Go `github.com/glebarez/sqlite` (cgo-free). Portable → Postgres later (P5) |
| Auth | **Server-side session** (bcrypt + `sessions` table + Gin middleware). **No JWT.** Instant revocation |
| CSRF | **`SameSite=Strict`** session cookie + an Origin/Referer check middleware. **No CSRF token** |
| TLS | Go-native **ACME/autocert** on :443 (prod); plain HTTP :8080 in dev (`APP_DEV=1`) |
| Images | `disintegration/imaging` — downscale ≤1600px + EXIF strip; JPEG/PNG/WebP in, JPEG out |
| Export | `xuri/excelize` + stdlib CSV |
| Frontend | **Next.js 15 static export** embedded via `//go:embed` (Node build-time only) |
| UI/styling | **Tailwind CSS v4 + shadcn/ui** (Radix), warm herbal theme + **light/dark toggle** (`next-themes`), vendored **`@fontsource/sarabun`** (body 300 / titles 500). All offline — no CDN |
| Service | **kardianos/service** (`internal/svc`) — runs under Windows SCM / Linux systemd / macOS launchd; `service install/uninstall/start/stop/restart` |
| Config | 12/15-Factor: env vars (`config.Load`), logs to stdout, graceful shutdown |
| Dev tooling | `docker-compose.dev.yaml`: web (`next dev` HMR) + api (`air`, Go 1.26) behind **nginx** on one origin, via **`docker compose watch`** (dev images under `deploy/dev/`) |
| CI/CD | GitHub Actions: **`ci.yml`** (go/web/e2e on push + PR) and **`release.yml`** (windows exe/msi + smoke + publish on `v*` tags) |

## 4. Repo layout

```
cmd/server/main.go          run | run (service) | service install/... | create-admin
internal/
  config db model           config, GORM open+Tx, GORM models + AutoMigrate
  auth                      password, session store, throttle, origin check, middleware, login/logout/current-user
  bootstrap clock httpx     first-admin seed, clock abstraction, JSON helpers + TLS server (ServeContext)
  svc                       run under host service manager (kardianos) + install/uninstall
  router webui              NewEngine (wires everything), embedded SPA + /media + JSON-404 fallback
  district user herb        CRUD repos + Gin handlers (central-admin managed)
  media                     image store (downscale/EXIF/streaming multipart) + usage bytes
  doctor recipe caserec     staff CRUD: consent gate, own-district, audit, code linking, ingredients
  publicapi export backup   public read (consent-filtered, PDPA-safe), staff export, nightly backup
web/                        Next.js: lib/{api,i18n,auth,crud,theme,utils}, app/globals.css (Tailwind v4 + theme tokens),
                            components/{CrudTable,CrudForm,IngredientEditor,PhotoUpload,ExportLinks} + components/ui/* (shadcn),
                            app/(staff|public)/*, e2e/* (+ e2e/fixtures/select.ts Radix helper)
deploy/windows/             WiX v5 MSI (phum-panya.wxs + .wixproj): installs server.exe + registers service
.github/workflows/          ci.yml (push/PR gates) + release.yml (tag → build exe/msi + publish)
Dockerfile docker-compose.yaml .env.example          # prod: single embedded binary (golang:1.26-alpine)
docker-compose.dev.yaml deploy/dev/* deploy/nginx/dev.conf .air.toml   # dev: web+api behind nginx (compose watch)
```

## 5. Build / run / deploy

**Build:** `make build` (needs Go + Node) → `./server` (single cgo-free binary; embeds the UI).
`make build-release` cross-compiles `bin/server-linux-amd64` + `bin/server.exe`.

**Run (dev):** `APP_DEV=1 APP_ADMIN_EMAIL=admin@example.com APP_ADMIN_PASSWORD=<pw> ./server`
→ plain HTTP on `:8080`. First run seeds the admin from those two env vars (idempotent).

**Run (prod):** set `APP_DOMAIN` (DNS → host, ports 80/443 open), drop `APP_DEV` → the binary
terminates TLS itself via Let's Encrypt.

**Run as a service** (Windows SCM / Linux systemd / macOS launchd):
```
server service install --admin-email=... --admin-password=... --domain=...
server service start | stop | uninstall
```
Data resolves under the service working dir: `%ProgramData%\phum-panya` (Windows) or
`/var/lib/phum-panya`. On Windows, the **MSI** does this for you: its wizard collects the admin
email/password + domain, installs `server.exe`, and registers + starts the service; uninstall
removes it. Silent: `msiexec /i phum-panya-<ver>-windows-amd64.msi /quiet ADMINEMAIL=... ADMINPASSWORD=... APPDOMAIN=...`.

**Docker:** `cp .env.example .env`, set `APP_ADMIN_PASSWORD`, `docker compose up --build`.
- Host port is `${APP_HOST_PORT:-8080}` (8080 can be reserved on Windows/WSL2 — set `APP_HOST_PORT=18080`).
- All state (DB, media, backups, ACME certs) lives in the `/data` volume.

**Dev stack (hot reload, split services):** `make dev`
(=`docker compose -f docker-compose.dev.yaml up -w --build --force-recreate`, needs
`APP_ADMIN_PASSWORD`). nginx routes `/api` + `/media` → the Go `api` service and everything else →
the Next.js `web` dev server (incl. HMR). Only nginx is published. Prod is untouched.

**Admin password** is seeded once into the volume. Change it later **in-app** (Staff → Users →
set password). `./server create-admin` is idempotent (won't overwrite an existing admin).

**Env vars:** `APP_HTTP_ADDR`, `APP_DOMAIN`, `APP_DB_PATH`, `APP_MEDIA_DIR`, `APP_BACKUP_DIR`,
`APP_DEV`, `APP_ADMIN_EMAIL`, `APP_ADMIN_PASSWORD` (+ compose `APP_HOST_PORT`). See `.env.example`.

**Test:** `go test ./...` and `cd web && npx playwright test` (Playwright builds+launches the
binary against a temp DB; needs chromium: `npx playwright install chromium`).

**Cut a release:** tag a commit that already passed `ci` and push it —
`git tag v1.1.0 && git push origin v1.1.0`. `release.yml` builds the linux binary, the Windows
`.exe`, and the `.msi` (smoke-testing the MSI install/register/uninstall), then publishes a
GitHub Release with all three. **Deploy to the VPS is still manual.**

**Restore a backup:** see `docs/ops/restore.md` (stop → unzip → place `app.db` + `media/` → start).

## 6. Key decisions (why it is the way it is)

- **Bilingual UI**, Thai default + English toggle — **labels only**; record content is never translated.
- **Session auth, admin-managed password resets, no email/SMTP** in P1.
- **CSRF = SameSite=Strict + Origin check, no token** (SRS NFR-SEC-6 satisfied by the cookie).
- **Embedded SQLite + local media + in-memory throttle + local backups** — deliberate 15-Factor
  deviations for a non-IT owner on one small VPS (documented in ADR-0001's 15-Factor section).
- **API routes are full-English** (project rule): `/api/current-user`, not `/api/me`.
- Images re-encoded to JPEG (drops EXIF/GPS); **HEIC input is out of P1** (pure-Go HEIC needs cgo).
- **Service = kardianos/service** (cross-platform, one dep); the MSI collects config in a wizard
  and bakes it into the service environment at install.

## 7. Security / PDPA posture (verified)

- Public API/export/media **never** expose `phone`, `consent_*`, or audit fields (explicit column
  projections, dedicated DTOs — no raw model serialization).
- **Consent gate** applied uniformly: only `consent_obtained=true` doctors and their
  recipes/cases appear publicly (JOIN-filtered, including nested cases).
- Own-district enforced on every staff write (create/update/delete/photo), including PUT's
  dual old+new district check.
- `/media` static serving is path-traversal-safe (`http.Dir`, live-tested across encodings).
- Reviews caught + fixed real defects: a **login timing side-channel** (account enumeration), a
  **case PUT cross-district re-parent bypass**, a **backup fd leak** + swallowed zip-close errors,
  and **photo-blanking-on-edit** data loss.

## 8. Known gaps / not done (all non-blocking for P1)

- Handlers lack `binding:"required"` tags → empty strings accepted (staff-entered, trusted data).
- Enum DB CHECKs absent for `Case.Result` / `Doctor.Status` / `User.Role` (Go-validated instead).
- `/api/current-user` omits `full_name` (staff dashboard shows role, not name).
- No role-based nav hiding: a district_editor sees 403 on admin-only Herbs/Users pages (backend
  correctly enforces; only a UX wart).
- No "export all districts" UI control for admin (backend supports it).
- No ESLint config (`tsc` + build are green).
- `Recipe.Photo` is a single string vs data-model §4.5 "image (many)"; recipes have no photo
  upload endpoint.
- FR-LINK-1 mismatch is surfaced at data-entry via `resolve-doctor` but not persisted as an
  admin-visible flag.
- **No CD to the VPS** — releases are built and published, but deployment to the server is manual.
- The MSI collects config in a wizard (validated end-to-end in CI); only `linux-amd64` /
  `windows-amd64` artifacts are built (no arm64 / macOS packages).

### Out of scope (later paid phases, per SRS §2)
- **P2** approval-before-publish + edit history · **P3** year snapshots/locking · **P4** bulk
  import of old paper/Excel · **P5** district-managed herb catalog + move to PostgreSQL.

## 9. Git state & next steps

- One trunk: **`main`** at `df39e76` (107 commits), `== origin/main`. No open feature branches.
- **`v1.0.0`** is tagged and released (linux binary + Windows `.exe` + `.msi`).
- **`ci.yml`** gates every push/PR (go/web/e2e); **`release.yml`** builds + publishes on `v*` tags.

**Immediate next steps:**
1. **Deploy `v1.0.0`** to the client VPS (single binary or the Docker image); set `APP_DOMAIN`,
   ports 80/443, seed the admin. Deployment is currently manual — decide whether to add a CD step.
2. **Run the SRS §6.1 UAT** with the client; capture real district/scale numbers to firm up the
   provisional performance targets.
3. Optional follow-ups (all non-blocking, see §8): role-based staff-nav hiding, `full_name` on
   `/api/current-user`, ESLint config, recipe multi-photo, and — when scale calls for it — the
   ADR-0001 SQLite→Postgres migration.
