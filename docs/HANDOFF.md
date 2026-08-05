# phum-panya — Handoff

Date: 2026-08-05 · Branch: **`main`** at `22acde9` · Last release tag: **`v1.1.0`** (P2–P5 + admin UI).
The four paid phases **P2–P5 are merged to `main`** (PRs #4–#7, all CI-green) **with their
frontend admin screens** (PR #8, `8cf3caa`), and the lot is **released as `v1.1.0`** (tag on
`6cde31b`; `release.yml` published the Windows exe/MSI + Linux binary). Since then, a
**post-`v1.1.0` hardening batch** and an **optional Postgres/Caddy container deploy stack**
(PR #35, ADR-0003) have merged to `main` but are **unreleased** — a `v1.2.0` tag would ship them.
`main` is one trunk — every feature branch is merged and deleted.

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

**P1 shipped as `v1.0.0`. P2–P5 (backend + admin UI) are built, green, merged to `main`, and
released as `v1.1.0`.**
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
- **P2–P5 now have frontend admin screens** (merged to `main` via PR #8, `8cf3caa`): approval
  queue, year locks, bulk import, herb merge/near-duplicate, plus role-gated nav. See §8.
- **Post-`v1.1.0` hardening batch** (merged to `main`, PRs #21–#26, #29, #31–#32): the parked
  follow-ups **#12–#18** plus two derived follow-ups (**#27, #28**) are all fixed — each on its own
  branch, TDD + fresh-context review, CI-green. Full suite is now **208 Go tests** (25 packages) +
  the Playwright e2e suite, all green. Only **#19** (CD/VPS auto-deploy) stays open. See §8.
- **Optional Postgres/Caddy container stack** (merged to `main` via PR #35, `22acde9`; **ADR-0003**,
  which complements — does not replace — ADR-0001). A **second, optional** deploy path for operators
  who prefer containers: a Compose stack of **Caddy + Next.js web + Go api + Postgres + a pg_dump
  backup sidecar**. The **single Go binary + SQLite stays the default and is unchanged** — all new
  behaviour is gated behind config (`APP_DB_DRIVER`, `APP_DATABASE_URL`, `APP_BEHIND_PROXY`,
  `APP_PUBLIC_ORIGIN`). DB is a GORM driver swap (pure-Go `pgx`, still cgo-free); Postgres backups
  are the pg_dump sidecar. Suite now **222 Go tests**; two new CI jobs (`go-postgres`,
  `stack-validate`). Runbook: `docs/ops/deploy-compose.md`. See §8.

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
Dockerfile.api web/Dockerfile docker-compose.yaml   # prod stack: Caddy+web+api+Postgres+backup (ADR-0003)
docker-compose.dev.yaml deploy/dev/                  # dev: web+api behind nginx, hot reload
.env.example
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
- **Docker (container stack, ADR-0003):** `cp .env.example .env`, set `APP_DOMAIN`,
  `APP_ADMIN_PASSWORD`, `POSTGRES_PASSWORD`, then `docker compose up -d --build` (needs ports
  80/443). Full runbook: `docs/ops/deploy-compose.md`. The single-binary Docker image was removed;
  the binary still ships via `make build` / `release.yml` (systemd / MSI).
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

**P2–P5 frontend admin screens are now built and merged** (PR #8, `8cf3caa`, CI-green).
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

**The parked follow-ups #12–#18 are now FIXED (post-`v1.1.0` hardening batch); #27/#28 too. Only
#19 remains open.**
- [#13](https://github.com/willywotz/phum-panya/issues/13) — **FIXED (PR #25).** `SetPhoto` now
  obeys P2 approval (editor change staged in a dedicated `pending_photo` column, admin applies at
  once), P3 year-lock (`guardYearWrite` on the Case path; Doctor has no `data_year`), and writes a
  `Revision`. Approve folds `pending_photo` into `photo` (composing with a pending content edit);
  reject discards it; the photo-only pending row shows in the review queue.
- [#18](https://github.com/willywotz/phum-panya/issues/18) — **FIXED (PR #26).** `Recipe.Photo` →
  a `RecipePhoto` child table (`ON DELETE CASCADE`), append endpoint `POST /api/recipes/:id/photo`,
  public projection returns a `photos` array, idempotent boot-time backfill of existing values.
- [#14](https://github.com/willywotz/phum-panya/issues/14) — **FIXED (PR #22).** herb `Merge`
  rejects self/chained/missing-canonical and re-points existing aliases; `herb.List()` and public
  `ListHerbs()` filter `alias_of_id IS NULL`.
- [#15](https://github.com/willywotz/phum-panya/issues/15) — **FIXED (PR #29).** `binding:"required"`
  /`oneof=` on request DTOs and `gorm:"check:..."` on enum columns. CHECKs apply on fresh
  AutoMigrate only (SQLite can't `ALTER`-add a CHECK) — the already-deployed DB relies on the
  binding guard; a one-time rebuild migration was judged not worth the data-loss risk.
- [#16](https://github.com/willywotz/phum-panya/issues/16) — **FIXED (PR #21).** `/api/current-user`
  returns `full_name`.
- [#12](https://github.com/willywotz/phum-panya/issues/12) — **FIXED (PR #23).** Districts + Users
  moved to admin-only nav + `RequireAdmin` on both pages; editors keep district-name reads in forms.
- [#17](https://github.com/willywotz/phum-panya/issues/17) — **FIXED (PR #24).** flat
  `eslint.config.mjs` (Next + TS preset); `lint` off deprecated `next lint`; lint wired into CI.
- [#27](https://github.com/willywotz/phum-panya/issues/27) — **FIXED (PR #31).** derived from #14:
  herb `mergeHandler` maps `ErrSelfMerge`/`ErrChainedMerge` → 400 and missing canonical → 404
  (was an opaque 500).
- [#28](https://github.com/willywotz/phum-panya/issues/28) — **FIXED (PR #32).** derived from #18:
  the public healer page reads the recipe `photos` array (renders every photo); the staff recipe
  screen gains photo upload via the append endpoint.
- [#19](https://github.com/willywotz/phum-panya/issues/19) — **OPEN.** no CD; the VPS deploy is
  manual. Two deploy targets now exist, both manual: the **single binary + SQLite** (default,
  runbook `docs/ops/deploy.md`) and the **optional Postgres/Caddy container stack** (ADR-0003,
  runbook `docs/ops/deploy-compose.md`). Parked pending a deploy-target + deploy-model decision
  (which target; push `deploy.yml` vs pull-based updater) + VPS secrets.

Not filed (self-remedying, documented behaviour): **P4** cases have no `code`, so re-importing a
file duplicates cases (coded entities are idempotent) — per-batch undo is the remedy; undo leaves
the append-only `Revision` rows. Not filed either: no per-worker e2e DB isolation (suite runs
serially) and the unformatted recipe-payload queue diff (frontend polish).
Role-based nav hiding is done (see the P2–P5 frontend note above).

### Out of scope (per the 2026-08-04 scoping decision)
- **SQLite as the default stays; no data migration tool.** The default binary keeps SQLite. Postgres
  is now a **supported optional runtime** via the compose stack (ADR-0003) for fresh installs, but
  making it the default, or migrating a running SQLite deployment into Postgres, is still out of
  scope. Re-scope trigger for the default: sustained list > 1 s / search > 2 s at real load, or
  `SQLITE_BUSY` write contention. GORM portability discipline is kept so both drivers run (ADR-0001).

## 9. Git state & next steps

- One trunk: **`main`** at `22acde9`, `== origin/main`. Clean; no open feature branches or worktrees.
- Merged since the last handoff: **PR #35** (`22acde9`) — the optional Postgres/Caddy container stack
  (ADR-0003). Earlier this session (hardening batch): PRs **#21** (#16), **#22** (#14), **#23** (#12),
  **#24** (#17), **#25** (#13), **#26** (#18), **#29** (#15), **#31** (#27), **#32** (#28), plus doc
  PRs **#30/#33/#34**. Before that: **#4–#7** (P2–P5 backend), **#8** (frontend), **#9–#11** (docs +
  release + deploy runbook), **#20** (issue links).
- **`v1.1.0`** is still the last tag (cut on `6cde31b`); the hardening batch **and** the optional
  container stack are **unreleased** on `main`. A **`v1.2.0`** tag would ship the lot.
- Suite: **222 Go tests** + Playwright e2e, all green. `ci.yml` now also runs `go-postgres` (live-PG
  integration) and `stack-validate` (compose config + caddy validate) on every push/PR;
  `release.yml` builds + publishes on `v*` tags.
- Open follow-up issues: **only #19** (CD/VPS auto-deploy — see §8). #12–#18, #27, #28 all closed.

**Immediate next steps:**
1. **Tag `v1.2.0`** to release the hardening batch + the optional container stack (or bundle it with
   the first VPS deploy).
2. **Choose the deploy target** ([#19](https://github.com/willywotz/phum-panya/issues/19)): the
   **single binary + SQLite** (default, `docs/ops/deploy.md`) or the **Postgres/Caddy container
   stack** (`docs/ops/deploy-compose.md`). Both are manual today. Then deploy to the client VPS and
   run the SRS §6.1 UAT on the live host (editor→pending→admin-approve→public flow).
3. **Decide #19's CD model** for the chosen target (push `deploy.yml` on release vs pull-based
   updater) and provide VPS secrets so CD can be built — it can't be built or verified headless
   without the host.
4. **Note on #15:** the enum CHECK constraints apply on fresh migrations only; the live `v1.1.0` DB
   won't gain them (SQLite can't `ALTER`-add a CHECK). `binding:oneof` guards all writes forward. If
   DB-level coverage on the existing DB is wanted, file a one-time rebuild-migration task.
