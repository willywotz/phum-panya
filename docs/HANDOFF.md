# phum-panya — Handoff

Date: 2026-08-05 · Branch: **`main`** · Last release tag: **`v1.0.0`** (P1).
The four paid phases **P2–P5 are merged to `main`** (PRs #4–#7, all CI-green) but **not yet
tagged as a new release**. `main` is one trunk — every feature branch is merged and deleted.

## 1. What this is

A web app to collect **folk-medicine records** (ตำรายาหมอพื้นบ้าน) for one province,
grouped by district (อำเภอ). The public reads everything; one editor per district plus a
central admin write the data. Access is by QR → browser. No app-store app, no map.

It ships as **one self-hosted Go binary** with the Next.js UI embedded and data in SQLite.

- Client context: `messages.txt`. Glossary/status: `CONTEXT.md`.
- Spec: `docs/superpowers/specs/2026-08-03-srs.md` (SRS, FR-*/NFR-*).
- Data model: `docs/superpowers/specs/2026-08-03-data-model-and-form-design.md`.
- Stack decision: `docs/adr/0001-single-go-binary-embedded-nextjs.md`.
- **P2 design decision: `docs/adr/0002-approval-on-row-pending-model-b.md`.**
- P1 plan (33 tasks): `docs/superpowers/plans/2026-08-03-p1-launch.md`.
- **P2–P5 scope: `docs/superpowers/plans/2026-08-04-p2-p5-scope.md`; build plan (~30 tasks):
  `docs/superpowers/plans/2026-08-04-p2-p5-implementation.md`.**
- Ops: `README.md`, `docs/ops/restore.md`.

## 2. Status

**P1 shipped as `v1.0.0`. P2–P5 are now built, green, and merged to `main` — backend only.**
Everything was built task-by-task with TDD + fresh-context review per task, plus an
independent whole-branch review before each merge. Each phase went out on its own branch →
PR → all CI green (Go + web type-check + Playwright e2e) → merged.

Paid phases delivered:

- **P2 — approval before publish + edit history** (PR #4). A district-editor save no longer
  publishes; it enters a **pending** state and the central admin approves it (Model B — every
  change is gated). Full append-only **`Revision`** history. Public reads require
  `review_state = 'approved'` **and** consent (independent gates). New packages `revision`,
  `review`; admin endpoints under `/api/review/...` (queue / approve / reject / approve-all).
  See ADR-0002.
- **P3 — year locking** (PR #5). A central admin freezes a whole `data_year`; its Recipe/Case
  rows become read-only (create/update refused for **all** writers → HTTP 409; editor delete
  refused; **admin delete = PDPA-erasure exemption**). A year locks only when its pending queue
  is empty. New `yearlock` package; `/api/year-locks`. Lock-only (no snapshot) — point-in-time
  reconstructs from the P2 `Revision` trail + the nightly backup.
- **P4 — bulk import** (PR #6). One canonical 4-sheet Excel template (Doctors / Recipes /
  Ingredients / Cases, linked by `code`) → parse → **dry-run validation report** → commit
  **through the domain services** (approved + logged, `BatchID`-tagged) → **per-batch undo**.
  New `ImportBatch` table + `importer` package; admin endpoints `POST /api/imports?dryRun=…`
  and `POST /api/imports/:batchId/undo`. Imported doctors default `consent=false` (hidden until
  consent recorded). Insert-only (existing codes skipped + reported).
- **P5 — district-managed herb catalog** (PR #7). The catalog stays **one shared list**, but
  write access widens: an editor may add herbs and edit ones its district created; only the
  admin merges/aliases across districts (re-points `Ingredient.HerbID`). `Herb` gains
  `CreatedByDistrictID` + `AliasOfID`; a save-time near-duplicate lookup nudges against dupes.

- **163 Go tests** (25 packages) + **14 Playwright e2e specs** (incl. the SRS §6.1 UAT, updated
  so the editor→pending→admin-approve flow is exercised). `go build`/`go vet` clean; cgo-free.
- **Two HIGH bugs were caught by the whole-branch reviews and fixed before merge** (see §7).
- **P2–P5 now have frontend admin screens** (branch `feat/p2-p5-frontend`, unmerged): approval
  queue, year locks, bulk import, herb merge/near-duplicate, plus role-gated nav. See §8.

## 3. Stack (as built)

| Layer | Choice |
|---|---|
| Runtime | One **Go** binary, `CGO_ENABLED=0` static, **`go 1.26`** |
| HTTP | **Gin** |
| DB | **SQLite** via **GORM** + pure-Go `github.com/glebarez/sqlite` (cgo-free). Portable → Postgres later (out of scope) |
| Auth | **Server-side session** (bcrypt + `sessions` table + Gin middleware). **No JWT.** Instant revocation |
| CSRF | **`SameSite=Strict`** session cookie + an Origin/Referer check middleware. **No CSRF token** |
| TLS | Go-native **ACME/autocert** on :443 (prod); plain HTTP :8080 in dev (`APP_DEV=1`) |
| Images | `disintegration/imaging` — downscale ≤1600px + EXIF strip; **JPEG/PNG/WebP only** (TIFF path blocked, NFR-IMG-1) |
| Import/Export | `xuri/excelize` (export **and** the P4 template parser) + stdlib CSV |
| Frontend | **Next.js 15 static export** embedded via `//go:embed` (Node build-time only) |
| UI/styling | **Tailwind CSS v4 + shadcn/ui** (Radix), warm herbal theme + **light/dark toggle**, vendored **Sarabun** font. All offline — no CDN |
| Service | **kardianos/service** (`internal/svc`) — Windows SCM / Linux systemd / macOS launchd |
| Config | 15-Factor (pragmatic): env vars (`config.Load`), logs to stdout, graceful shutdown |
| Dev tooling | `docker-compose.dev.yaml`: web (`next dev` HMR) + api (`air`) behind **nginx**, via **`docker compose watch`** |
| CI/CD | GitHub Actions: **`ci.yml`** (go/web/e2e on push + PR) and **`release.yml`** (windows exe/msi + smoke + publish on `v*` tags) |

## 4. Repo layout

```
cmd/server/main.go          run | run (service) | service install/... | create-admin
internal/
  config db model           config, GORM open+Tx, GORM models + AutoMigrate (9 entities)
  auth                      password, session store, throttle, origin check, middleware, login/logout/current-user
  bootstrap clock httpx      first-admin seed, clock abstraction, JSON helpers + TLS server (ServeContext)
  svc                       run under host service manager (kardianos) + install/uninstall
  router webui              NewEngine (wires everything), embedded SPA + /media + JSON-404 fallback
  district user herb        CRUD repos + Gin handlers; herb now widened to district writers + merge/alias (P5)
  media                     image store (downscale/EXIF/streaming multipart, JPEG/PNG/WebP allowlist) + usage bytes
  doctor recipe caserec     staff CRUD: consent gate, own-district, audit, code linking, ingredients,
                            role-branched write path (editor→pending / admin→immediate, P2), year-lock guard (P3)
  revision                  append-only edit history log (P2)
  review                    central-admin approval queue: queue/approve/reject/approve-all (P2)
  yearlock                  lock/unlock/list a data_year + write guard (P3)
  importer                  canonical template parse → dry-run report → commit via domain services → undo (P4)
  publicapi export backup   public read (consent + review_state filtered, PDPA-safe), staff export, nightly backup
web/                        Next.js SPA (staff + public) — NOTE: no UI yet for the P2–P5 admin flows (see §8)
deploy/windows/             WiX v5 MSI: installs server.exe + registers service
.github/workflows/          ci.yml (push/PR gates) + release.yml (tag → build exe/msi + publish)
Dockerfile docker-compose*.yaml .env.example   # prod: single embedded binary; dev: web+api behind nginx
```

## 5. New API surface (P2–P5, all admin-gated unless noted)

```
# P2 review queue (central_admin)
GET  /api/review/queue
POST /api/review/entry/:entityType/:entityId/approve      # entityType = doctor|recipe|case
POST /api/review/entry/:entityType/:entityId/reject       # body: {"reason": "..."}
POST /api/review/doctor/:doctorId/approve-all             # bulk-approve a doctor's tree

# P3 year locks (central_admin)
GET    /api/year-locks
POST   /api/year-locks                                    # body: {"dataYear": 2567}
DELETE /api/year-locks/:dataYear

# P4 bulk import (central_admin, multipart .xlsx field "file")
POST /api/imports?dryRun=true|false
POST /api/imports/:batchId/undo

# P5 herb catalog
POST /api/herbs                                           # RequireAuth (editor or admin) — was admin-only
PUT  /api/herbs/:id                                       # RequireAuth; editor may edit only own-district herbs (403 else)
GET  /api/herbs/near-duplicates?thaiName=...              # RequireAuth — save-time warning
POST /api/herbs/:id/merge/:canonicalId                    # central_admin — alias :id to :canonicalId, re-point ingredients
```

Behavioural change to existing writes: a **district-editor** create/update/delete on
Doctor/Recipe/Case now enters the P2 pending queue instead of publishing; a **central-admin**
write is immediate + logged. The public API hides anything not `review_state = 'approved'`.

## 6. Build / run / deploy

Unchanged from `v1.0.0` — see git history / README. Quick reference:

- **Build:** `make build` (Go + Node) → `./server` (single cgo-free binary; embeds the UI).
- **Run (dev):** `APP_DEV=1 APP_ADMIN_EMAIL=… APP_ADMIN_PASSWORD=… ./server` → HTTP `:8080`.
- **Run (prod):** set `APP_DOMAIN` (ports 80/443), drop `APP_DEV` → Go-native Let's Encrypt TLS.
- **Service:** `server service install --admin-email=… --admin-password=… --domain=…`; or the
  Windows **MSI** wizard.
- **Docker:** `cp .env.example .env`, set `APP_ADMIN_PASSWORD`, `docker compose up --build`
  (all state in `/data`; set `APP_HOST_PORT` if 8080 is reserved).
- **Dev stack (hot reload):** `make dev`.
- **Test:** `go test ./...` and `cd web && npx playwright test` (needs `npx playwright install chromium`).
- **Env vars:** `APP_HTTP_ADDR`, `APP_DOMAIN`, `APP_DB_PATH`, `APP_MEDIA_DIR`, `APP_BACKUP_DIR`,
  `APP_DEV`, `APP_ADMIN_EMAIL`, `APP_ADMIN_PASSWORD` (+ compose `APP_HOST_PORT`).
- **Restore a backup:** `docs/ops/restore.md`.

**Migration note:** the new columns/tables are applied by GORM `AutoMigrate` on startup
(`review_state`/`pending_json`/`pending_delete`/`rejection_reason` on Doctor/Recipe/Case with
`default:approved` so existing rows stay public; `revision`, `year_locks`, `import_batches`
tables; `Herb` provenance/alias columns; `batch_id` tags). No manual migration step.

## 7. Security / PDPA posture (verified)

- Public API/export/media **never** expose `phone`, `consent_*`, audit, `pending_json`,
  `rejection_reason`, or `Revision.after_json` (explicit column projections / DTOs).
- **Two independent public gates**: `consent_obtained = true` AND `review_state = 'approved'`,
  applied in one place (`publicapi`) on every doctor/recipe/case read (JOIN-filtered, nested
  cases included). The final P2 review confirmed no public path bypasses the review gate.
- **Approval authority**: only `central_admin` reaches the review/lock/import/merge endpoints;
  a district-editor's writes queue and can never self-publish or self-approve. Herb ownership
  can't be hijacked (provenance is stamped server-side and is immutable through update).
- Own-district enforced on every staff write (incl. PUT's dual old+new district check).
- **Whole-branch reviews caught + fixed two HIGH defects before merge:**
  - **P3** — a cross-year pending edit could be approved into a year that was locked *after* the
    edit was queued (the lock precondition only scanned real `data_year` columns). Fixed by
    re-checking the target year at approve time.
  - **P4** — `Undo` deleting a batch's doctor cascade-deleted **other** batches'/manual
    recipes/cases via the FK. Fixed so undo removes only childless batch doctors.
  Both were reproduced by the reviewer and confirmed red→green.

## 8. Known gaps / not done

**P2–P5 frontend admin screens are now built** (branch `feat/p2-p5-frontend`, unmerged, CI-green).
- Staff UI now exists for: the **approval queue** (`/staff/review` — approve/reject/approve-all,
  with a per-item current-vs-proposed diff via the new `GET /api/review/entry/:entityType/:entityId`
  detail endpoint), **year locks** (`/staff/year-locks`), **bulk import** (`/staff/imports` —
  upload + dry-run report + commit + undo), and **herb merge/near-duplicate** (on `/staff/herbs`).
  Nav is role-gated (admin links hidden from editors; `RequireAdmin` guards the pages), which also
  closes the P1 carry-over "no role-based nav hiding" gap. One Playwright spec per screen; the
  full 19-case e2e suite is green (now run serially — `workers: 1` — because the single shared
  SQLite dev DB makes parallel write-heavy specs flaky). The UAT e2e still drives approval via the
  API as before.
- Parked frontend follow-ups: no per-worker e2e DB isolation (the reason the suite runs serially);
  the queue diff renders a recipe `{recipe,ingredients}` pending payload as one unformatted cell.

Smaller, parked follow-ups (non-blocking):
- **P2:** `SetPhoto` bypasses the approval/lock gates — a photo swap on an approved (or locked)
  row is immediate and unlogged. (Row content edits are correctly gated; only photos aren't.)
- **P3:** `SetPhoto` is likewise not year-lock-guarded.
- **P4:** cases have no `code`, so re-importing a file duplicates cases (coded entities are
  idempotent); per-batch undo is the remedy. Undo leaves the append-only `Revision` rows.
- **P5:** a merged alias herb still appears in the admin + public catalog (no `alias_of_id IS
  NULL` filter) — recipes resolve to the canonical herb, so this is cosmetic; `Merge` has no
  self/chained-merge guard (admin-only tool).
- Carried over from P1: no `binding:"required"` tags; no enum DB CHECKs; `/api/current-user`
  omits `full_name`; no ESLint config; `Recipe.Photo` single-string;
  **no CD to the VPS** (releases build + publish, deploy is manual).
  (Role-based nav hiding is now done — see the P2–P5 frontend note above.)

### Out of scope (per the 2026-08-04 scoping decision)
- **SQLite → PostgreSQL migration** — explicitly out of scope; the app stays on SQLite. Re-scope
  trigger: sustained list > 1 s / search > 2 s at real load, or `SQLITE_BUSY` write contention.
  GORM portability discipline is kept so it stays a driver swap (see the scope doc + ADR-0001).

## 9. Git state & next steps

- One trunk: **`main`** at `00e2694` (144 commits), `== origin/main`. No open feature branches.
- PRs **#4 (P2), #5 (P3), #6 (P4), #7 (P5)** merged. `v1.0.0` is the last tag; P2–P5 are on
  `main` but **unreleased** (no new tag cut).
- `ci.yml` gates every push/PR; `release.yml` builds + publishes on `v*` tags.

**Immediate next steps:**
1. **Decide whether P2–P5 are accepted** — the scope doc marked them "not funded." The
   **frontend admin screens are now built** (branch `feat/p2-p5-frontend`, §8); once that branch is
   reviewed and merged, cut a release tag (e.g. `v1.1.0`) after a UAT pass on the new screens.
2. **Deploy** to the client VPS (still manual) and run the updated SRS §6.1 UAT — now including
   the editor→pending→admin-approve→public flow.
3. Optional hardening: gate `SetPhoto` through the P2/P3 rules; add self/chained-merge guards to
   herb `Merge`; the P1 carry-over polish in §8.
