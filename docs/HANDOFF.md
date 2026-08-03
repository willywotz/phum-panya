# phum-panya — Handoff

Date: 2026-08-04 · Branch: `feat/p1-launch` (53 commits ahead of `main`, **unmerged**)

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

**P1 is functionally complete and green**, built task-by-task with TDD + fresh-context
review per task and a clean whole-branch final review.

- **107 Go tests** (20 packages) + **7 Playwright e2e specs** (incl. the full SRS §6.1 UAT).
- cgo-free single binary via `make build`; Docker image builds and runs (health, embedded
  UI, seeded-admin login verified in-container).
- **Not merged** to `main`; `main` == `origin/main` at the pre-build docs commit.

## 3. Stack (as built)

| Layer | Choice |
|---|---|
| Runtime | One **Go** binary, `CGO_ENABLED=0` static, `go 1.25` |
| HTTP | **Gin** |
| DB | **SQLite** via **GORM** + pure-Go `github.com/glebarez/sqlite` (cgo-free). Portable → Postgres later (P5) |
| Auth | **Server-side session** (bcrypt + `sessions` table + Gin middleware). **No JWT.** Instant revocation |
| CSRF | **`SameSite=Strict`** session cookie + an Origin/Referer check middleware. **No CSRF token** |
| TLS | Go-native **ACME/autocert** on :443 (prod); plain HTTP :8080 in dev (`APP_DEV=1`) |
| Images | `disintegration/imaging` — downscale ≤1600px + EXIF strip; JPEG/PNG/WebP in, JPEG out |
| Export | `xuri/excelize` + stdlib CSV |
| Frontend | **Next.js 15 static export** embedded via `//go:embed` (Node build-time only) |
| Config | 12/15-Factor: env vars (`config.Load`), logs to stdout, graceful shutdown |

## 4. Repo layout

```
cmd/server/main.go          run | create-admin ; opens DB, migrates, seeds admin, backup ticker, Serve
internal/
  config db model           config, GORM open+Tx, GORM models + AutoMigrate
  auth                      password, session store, throttle, origin check, middleware, login/logout/current-user
  bootstrap clock httpx     first-admin seed, clock abstraction, JSON helpers + TLS server
  router webui              NewEngine (wires everything), embedded SPA + /media + JSON-404 fallback
  district user herb        CRUD repos + Gin handlers (central-admin managed)
  media                     image store (downscale/EXIF/streaming multipart) + usage bytes
  doctor recipe caserec     staff CRUD: consent gate, own-district, audit, code linking, ingredients
  publicapi export backup   public read (consent-filtered, PDPA-safe), staff export, nightly backup
web/                        Next.js: lib/{api,i18n,auth,crud}, components/{CrudTable,CrudForm,IngredientEditor,PhotoUpload,ExportLinks}, app/(staff|public)/*, e2e/*
Dockerfile docker-compose.yaml .env.example
```

## 5. Build / run / deploy

**Build:** `make build` (needs Go + Node) → `./server` (single cgo-free binary; embeds the UI).

**Run (dev):** `APP_DEV=1 APP_ADMIN_EMAIL=admin@example.com APP_ADMIN_PASSWORD=<pw> ./server`
→ plain HTTP on `:8080`. First run seeds the admin from those two env vars (idempotent).

**Run (prod):** set `APP_DOMAIN` (DNS → host, ports 80/443 open), drop `APP_DEV` → the binary
terminates TLS itself via Let's Encrypt.

**Docker:** `cp .env.example .env`, set `APP_ADMIN_PASSWORD`, `docker compose up --build`.
- Host port is `${APP_HOST_PORT:-8080}` (8080 can be reserved on Windows/WSL2 — set
  `APP_HOST_PORT=18080` there).
- All state (DB, media, backups, ACME certs) lives in the `/data` volume.

**Admin password** is seeded once into the volume. Change it later **in-app** (Staff → Users →
set password), not via `APP_ADMIN_PASSWORD` (which only seeds when no admin exists). The
`./server create-admin` subcommand is also idempotent (won't overwrite an existing admin).

**Env vars:** `APP_HTTP_ADDR`, `APP_DOMAIN`, `APP_DB_PATH`, `APP_MEDIA_DIR`, `APP_BACKUP_DIR`,
`APP_DEV`, `APP_ADMIN_EMAIL`, `APP_ADMIN_PASSWORD` (+ compose `APP_HOST_PORT`). See `.env.example`.

**Test:** `go test ./...` and `cd web && npx playwright test` (Playwright builds+launches the
binary against a temp DB; needs chromium: `npx playwright install chromium`).

**Restore a backup:** see `docs/ops/restore.md` (stop → unzip → place `app.db` + `media/` → start).

## 6. Key decisions (why it is the way it is)

Settled during a grilling session (see git history / SRS):
- **Bilingual UI**, Thai default + English toggle — **labels only**; record content is never translated.
- **Session auth, admin-managed password resets, no email/SMTP** in P1.
- **CSRF = SameSite=Strict + Origin check, no token** (SRS NFR-SEC-6 satisfied by the cookie).
- **Embedded SQLite + local media + in-memory throttle + local backups** — deliberate 15-Factor
  deviations for a non-IT owner on one small VPS (documented in ADR-0001's 15-Factor section).
- **API routes are full-English** (project rule): `/api/current-user`, not `/api/me`.
- Images re-encoded to JPEG (drops EXIF/GPS); **HEIC input is out of P1** (pure-Go HEIC needs cgo).

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

## 8. Known gaps / not done

### ⚠️ Biggest gap: NO visual styling / CSS
The frontend is **semantic HTML with essentially no screen CSS** — only `print.css`
(`@media print`). No `globals.css`, no Tailwind/CSS framework, near-zero `className` use. It is
**functional and accessible but visually bare** (browser defaults). This was never in the
SRS/plan (scope was function + best-effort a11y, no design task). A styling pass is the top
follow-up. Because the markup is clean semantic HTML, a classless CSS framework (e.g. Pico.css)
or a hand-rolled `globals.css` would style it with minimal component changes.

### Deferred minors (from the final whole-branch review — all non-blocking for P1)
- Handlers lack `binding:"required"` tags → empty strings accepted (staff-entered, trusted data).
- Enum DB CHECKs absent for `Case.Result` / `Doctor.Status` / `User.Role` (Go-validated instead).
- `/api/current-user` omits `full_name` (staff dashboard shows role, not name).
- No role-based nav hiding: a district_editor sees 403 on admin-only Herbs/Users pages (backend
  correctly enforces; only a UX wart).
- Storage `<progress>` bar lacks an aria-label.
- No "export all districts" UI control for admin (backend supports it).
- Backup `MkdirAll` error is silently discarded (no log line).
- No ESLint config (`tsc` + build are green).
- `Recipe.Photo` is a single string vs data-model §4.5 "image (many)"; recipe photo upload is in
  no endpoint (recipes have no photo upload path at all).
- FR-LINK-1 mismatch is surfaced at data-entry via `resolve-doctor` but not persisted as an
  admin-visible flag.

### Out of scope (later paid phases, per SRS §2)
- **P2** approval-before-publish + edit history · **P3** year snapshots/locking · **P4** bulk
  import of old paper/Excel · **P5** district-managed herb catalog + move to PostgreSQL.

## 9. Git state & next steps

- Everything is on **`feat/p1-launch`** (`e31090c`), 53 commits ahead of `main`, **unmerged**,
  **unpushed**. `main` == `origin/main` at the pre-build docs commit (`03be3c4`).
- The SDD ledger workspace was deleted after the clean final review — history lives in git
  (`git log --oneline main..HEAD`); each commit maps to a reviewed task.

**Immediate next steps:**
1. Decide integration: merge `feat/p1-launch` → `main`, or open a PR.
2. **Add the styling pass** (§8) — the one thing between "works" and "looks launch-ready".
3. Run the SRS §6.1 UAT with the client; capture real district/scale numbers to firm up the
   provisional performance targets.
