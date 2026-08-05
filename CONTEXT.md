# CONTEXT

## Project

A web app to collect folk-medicine records (ตำรายาหมอพื้นบ้าน) for one province,
grouped by district (อำเภอ). The public reads all data. One editor per district plus a
central admin write the data. No mobile-store app; users open the site by QR scan. No map.

Client chat: `messages.txt`.
Reference app (client-forwarded): a Thai "Tok Bidan" herbal app, but without the map.

## Status

- Design phase.
- Data model + standard fill-in form: designed and approved.
  See `docs/superpowers/specs/2026-08-03-data-model-and-form-design.md`.
- Feature research: done. Data model has no missing fields; gaps are operational/ethical.
  See `docs/research/2026-08-03-feature-research.md`.
  Top adds: healer consent + attribution, edit history, print/PDF, bulk export, feedback form.
- Paranoid review (fresh-context verifier): done. Fixes applied to model, form, and research:
  - Consent/PDPA: added `consent_obtained`/`consent_date` on Doctor, opt-out rule, form
    consent box, and a "must-do before launch" section (spec §3.1).
  - Record linking: added `code` on Doctor and Recipe; the form links by code, not name.
  - Year rule: one Doctor row per healer with `first_year`; Recipe/Case keep `data_year` (§3.2).
  - Herb fallback: a pending-herb name so district entry is never blocked.
  - Research file: corrected the backup cost to real scale; fixed 3 mislabeled citations.
- SRS: written and approved. See `docs/superpowers/specs/2026-08-03-srs.md`.
  Build-facing, English (STE). Covers all phases (P1 launch → P5), each requirement phase-tagged.
  Stack locked in `docs/adr/0001-single-go-binary-embedded-nextjs.md`:
  one Go binary + embedded Next.js static export; SQLite → PostgreSQL (portable SQL,
  performance-triggered); Go-native ACME/TLS (client provides the domain).
  Key P1 decisions: bilingual UI (Thai default + English label toggle, content untranslated);
  session login with admin-managed resets (no SMTP); server-side image downscale + EXIF strip,
  no input cap; nightly single-zip backup (DB + images) + restore docs; print/PDF public page
  + staff-only bulk export; best-effort availability (no SLA); WCAG 2.1 AA basics.
- P1 implementation plan: written, reviewed, and revised (v2). See
  `docs/superpowers/plans/2026-08-03-p1-launch.md`. One monolithic P1 plan, 33 TDD tasks in 8 parts
  (foundation → models → auth → users/districts/herbs/media → doctor/recipe/case → public/export/backup
  → Next.js frontend → UAT). Each task is test-first with real Go/TS code.
- Stack (revised after a pre-execution verifier review): **Gin + GORM** with the **pure-Go
  `glebarez/sqlite`** driver (cgo-free), `autocert` TLS, `disintegration/imaging`, `xuri/excelize`;
  Next.js 15 static export embedded via `embed.FS`. Auth = **server-side session** (bcrypt + session
  row + Gin middleware), **no JWT**; CSRF defense = **`SameSite=Strict` cookie + Origin check, no CSRF
  token**. ADR-0001 updated to match.
- Pre-execution review (obs 13325): 15 findings, 2 Critical. All folded into plan v2, marked [FIX-n]:
  first-admin bootstrap (Task 6), Next.js `generateStaticParams`→[], `Serve` error handling, GORM tx
  (no MaxOpenConns(1) deadlock), `media` pkg rename, four stub tests given real assertions, embed
  placeholder tracked, single Playwright webServer, streaming multipart, consent filter on the recipe
  path. Recorded deviation: HEIC input out of P1 (needs cgo).
- P1 build: **complete** on branch `feat/p1-launch` (49 commits, subagent-driven TDD).
  All 33 plan tasks + a gap-remediation (public ingredients/media/districts) done; each
  passed a fresh-context review. Suite: **107 Go tests + 11 Playwright**, cgo-free single
  binary via `make build`; full SRS §6.1 UAT e2e passes. See `README.md` and
  `docs/ops/restore.md`.
  - Reviews caught and fixed real defects: login timing side-channel (account enumeration),
    a case PUT cross-district re-parent bypass, a backup fd leak + swallowed zip-close errors,
    and a photo-blanking-on-edit data loss (doctor + case). PDPA verified end-to-end
    (public API/export/media never expose phone/consent/audit; consent filter on all paths).
  - Stack as built: Gin + GORM (`glebarez/sqlite`, cgo-free) + bcrypt server-side sessions +
    `SameSite=Strict`/Origin CSRF defense; Next.js static export embedded; Go-native TLS.
  - 15-Factor applied in spirit (ADR-0001 deviations); API routes are full-English
    (`/api/current-user`, not `/api/me`).
  - Deferred polish (non-blocking, see SDD ledger): input `binding:required` tags, enum DB
    CHECKs, role-based staff-nav hiding, `full_name` on `/api/current-user`,
    ESLint config, Recipe multi-photo. (storage-meter aria-label: closed in styling pass.)
- Containerized: `Dockerfile` (multi-stage: Node builds the Next.js export → Go embeds it →
  cgo-free static binary on alpine + ca-certificates) + `docker-compose.yaml` (one `app`
  service, `/data` volume for DB/media/backups/certs, required `APP_ADMIN_PASSWORD`, dev
  plain-HTTP:8080 by default, commented prod TLS on 80/443). Image builds and runs; health,
  embedded UI, and seeded-admin login verified in-container.
- Styling pass: **complete** on branch `feat/styling-pass` (off `feat/p1-launch`, 8 TDD tasks,
  subagent-driven build→verify). Adds **Tailwind CSS v4 + shadcn/ui** (Radix), a **warm herbal
  theme** with a **light/dark toggle** (`next-themes`), a real **landing page** (hero + browse
  cards), and a **vendored Thai font** (`@fontsource/sarabun`, weights 300/500/700; body 300,
  titles 500) — no CDN, so the single
  cgo-free binary and static export are unchanged. Shared `CrudForm`/`CrudTable`/`IngredientEditor`
  now use shadcn controls: single-selects are Radix `Select`, the add/edit form is a modal `Dialog`,
  delete is an `AlertDialog`; the e2e specs moved to role-based drivers (`selectByName` combobox
  helper, `alertdialog`). Gate green: no-CDN grep clean, `web/out/` builds, **107 Go tests**,
  **13 Playwright specs**, `tsc` clean; print rule (`.no-print` nav) kept. See
  `docs/superpowers/plans/2026-08-04-styling-pass.md`.
  - Follow-up done: the four hand-rolled staff pages (`cases`, `herbs`/ReconcilePanel, `doctors`,
    `recipes`) were fully migrated too — all 11 remaining native `<select>` became Radix `Select`
    and their raw inputs/buttons became shadcn. Only the doctor `specialty` `<select multiple>`
    stays native (Radix has no multiselect). No native single-select `<select>` remains in the app.
- Dev stack (branch `feat/dev-compose-nginx`, off `feat/p1-launch`): `docker-compose.dev.yaml`
  runs the frontend and API as **separate** containers behind an **nginx** reverse proxy on one
  origin — `nginx` (`nginx:alpine`) routes `/api` + `/media` → Go `api` service, everything else
  → Next.js `web` (`next dev`, HMR). Uses **`docker compose watch`** (not bind mounts): each
  service builds from a small dev image under `deploy/dev/` (source + deps baked in) and
  `develop.watch` syncs later changes — `sync` for source, `rebuild` on lockfile/go.mod change,
  `sync+restart` for the nginx conf. Hot reload both sides: `web` = `next dev`
  (`WATCHPACK_POLLING`); `api` = `air` live-reload (v1.67.4) on `golang:1.26-alpine`. Only nginx
  is published (`${APP_HOST_PORT:-8080}:80`). No app-code changes — the frontend's relative
  `fetch('/api/...')` + the dev-mode no-op CSRF check make same-origin proxying work as-is.
  Prod (single embedded binary, `docker-compose.yaml`) is untouched. Smoke-tested end-to-end:
  health, landing, HMR websocket 101, auth 401 all via nginx, plus live `watch` sync verified
  (web file synced ~2s, Go edit synced + air rebuilt). Run with `make dev`
  (=`docker compose -f docker-compose.dev.yaml up -w --build --force-recreate`). See
  `docs/superpowers/specs/2026-08-04-dev-compose-nginx-design.md`.
- Go bump (branch `chore/bump-go-1.26`): `go.mod` `go 1.25.0` → `go 1.26.0` and the prod
  `Dockerfile` build stage `golang:1.25-alpine` → `golang:1.26-alpine` (matches the dev API
  image). Build and all **107 Go tests** stay green.
- Font change (branch `feat/font-sarabun`, off `feat/p1-launch`): swapped the vendored Thai font
  from Noto Sans Thai to **Sarabun** (`@fontsource/sarabun`, weights 300/500/700). Base `body`
  now weight **300** (`font-light`); `h1`/`h2` now weight **500** (`font-medium`). Still no CDN
  (Thai subset bundled). Gate: `tsc` clean, `web/out/` builds, Sarabun woff2 in output, no stray
  Noto refs.
- CI/CD (GitHub Actions), split into two workflows:
  - **`ci.yml`** — on push (`main`, `feat/**`) + PR to `main`, three jobs run every gate: **go**
    (`go build`/`go test`), **web** (`tsc` + static export), **e2e** (Playwright drives the real
    binary via the config's `make build` webServer).
  - **`release.yml`** — on `v*` tags only: a **windows** job builds `server.exe` + the WiX MSI and
    smoke-tests its install/register/uninstall, then a **release** job builds the linux binary and
    publishes a GitHub Release with all three assets. Tag a commit that already passed `ci`. Deploy
    to the VPS stays manual.
- Windows service + MSI (branch `feat/windows-service-msi`): the binary now runs under the host
  service manager via **kardianos/service** (`internal/svc`) — subcommands `service
  install|uninstall|start|stop|restart` (Windows SCM / Linux systemd / macOS launchd). `httpx.Serve`
  refactored to `ServeContext(ctx,…)` so the supervisor can stop it. `service install` takes
  `--admin-email/--admin-password/--domain` and bakes them into the service env; data lives under
  `%ProgramData%\phum-panya` (Windows) or `/var/lib/phum-panya`. Makefile `build-release`
  cross-compiles `bin/server-linux-amd64` + `bin/server.exe` (cgo-free). A WiX v5 installer
  (`deploy/windows/phum-panya.{wxs,wixproj}`) collects admin email/password/domain in a wizard page
  and registers+starts the service on install, stops+removes on uninstall. CI gains a **windows**
  job (build exe+msi, silent install/query/uninstall smoke test) and the **release** job now
  attaches the linux binary + `.exe` + `.msi` on `v*` tags. 113 Go tests green.
- **Shipped**: all work merged to `main` (107 commits) and released as **`v1.0.0`** (PR #1 merged;
  PR #2 split CI into `ci.yml` + `release.yml`). The release has three assets — linux binary,
  Windows `.exe`, and `.msi`. One trunk; all feature branches deleted. 114 Go tests green.
- Next step: **deploy `v1.0.0`** to the client VPS (deployment is manual — no CD yet), then run the
  SRS §6.1 UAT with the client. See `docs/HANDOFF.md` for the full picture.
- **Paid phases scoped + design settled** (grilling, 2026-08-04):
  `docs/superpowers/plans/2026-08-04-p2-p5-scope.md`. Engineering-readiness depth — core decisions
  settled so each phase can start a TDD task plan when funded. P2 (approval + edit history): every
  editor save goes pending, public keeps last-approved (Model B), per-record, consent+review as
  independent gates, on-row pending state + append-only Revision, admin writes immediate,
  reject-returns-with-reason. P3 (year locking): lock-only, Recipe/Case only (Doctor exempt),
  erasure overrides. P4 (bulk import): canonical Excel template, imported rows approved+consent-gated,
  insert-only skip-and-report. P5 (district herb catalog): one shared catalog, add+edit-own +
  admin-merge. **SQLite→Postgres migration moved to OUT OF SCOPE** (re-scope only if list >1s /
  search >2s at real load, or SQLITE_BUSY contention). Not funded.

- **Security (Dependabot)**: cleared 6 alerts. npm (5) fixed via `web/package.json` `overrides`
  (`postcss ^8.5.23`, `sharp ^0.35.0`) — both were transitive via Next 15; resolved to postcss
  8.5.25 + sharp 0.35.3, `npm audit` clean, `tsc` + static export green. `shadcn` (scaffolding CLI,
  no runtime import) moved to devDependencies. Go (1): `disintegration/imaging` crafted-TIFF crash
  has no upstream patch, so `internal/media/store.go` now sniffs content-type and accepts only
  JPEG/PNG/WebP (NFR-IMG-1), making the TIFF decode path unreachable (TDD: `TestSaveReaderRejectsTIFF`).
  115 Go tests green.

- **P2 — approval before publish + edit history (in progress, branch `feat/p2-approval-history`)**:
  district-editor writes no longer publish at once — every change enters a `pending` state and a
  central admin approves it before the public sees it (Model B). Central-admin writes stay immediate
  (auto-approved + logged). On-row pending model: Doctor/Recipe/Case gain `review_state`,
  `pending_json` (edit overlay), `pending_delete`, `rejection_reason`; a new append-only `Revision`
  table stores one snapshot per approved change (+ reject events). Public reads add
  `review_state = 'approved'` next to the consent gate, in one place (`internal/publicapi`). New
  packages: `revision` (history log), `review` (queue + approve/reject/bulk, endpoints under
  `/api/review/...`). Design note: revision `Append` runs after the write tx commits (a second
  pooled connection inside the tx deadlocks SQLite's WAL writer). See `docs/adr/0002`. 130 Go tests
  green.

- **P3 — year locking (in progress, branch `feat/p3-year-locking`)**: a central admin can freeze a
  whole `data_year` so its Recipe/Case rows become read-only. New `YearLock` table
  (`data_year` pk, `locked_at`, `locked_by`) + `yearlock` package (`Lock`/`Unlock`/`List`/`IsLocked`,
  admin-only endpoints under `/api/year-locks`). A year locks only when its pending queue is empty
  (so "locked" = final approved state). **Write guard** in recipe/caserec: create + update are
  refused for ALL writers when the row's `data_year` is locked (returns HTTP 409); an editor's
  queued delete is refused; an **admin delete is the PDPA-erasure exemption and is always allowed**.
  Design note (refines the plan): admin edits/creates into a locked year are refused too — a locked
  year is read-only, and only erasure/unpublish overrides the freeze; this also makes P4 imports into
  a locked year refused for free. The approval path also re-checks the target `data_year` at approve
  time, so a cross-year pending edit cannot be promoted into a year that was locked after the edit was
  queued. Not guarded (known follow-up): `SetPhoto` photo swaps. Lock-only —
  no materialized snapshot; point-in-time state, if ever needed, reconstructs from the P2 `Revision`
  trail + the nightly backup. 141 Go tests green.

- **P4 — bulk import (in progress, branch `feat/p4-bulk-import`)**: load client data in bulk from one
  canonical Excel template through the same domain services and rules as manual entry. New
  `ImportBatch` table + `BatchID` tags on Doctor/Recipe/Case. New `importer` package: a 4-sheet
  template (Doctors/Recipes/Ingredients/Cases, linked by `code`; Ingredients is a linked sheet keyed
  by `recipe_code`) → parse → **dry-run validation report** (required fields, duplicate-code skips,
  doctor/recipe link resolution, district existence, locked-year refusal) → **commit** through the
  domain services (`immediate` = approved + logged), tagging `BatchID`. Admin-only endpoints:
  `POST /api/imports?dryRun=true|false` (multipart .xlsx), `POST /api/imports/:batchId/undo`. Imported
  doctors default to `consent_obtained = false` (hidden until consent recorded). Insert-only: existing
  `code`s are skipped + reported (idempotent for coded entities). Unknown herbs take the pending-herb
  path. Design note: the commit is NOT one DB transaction (domain repos are pool-bound and would
  deadlock the WAL writer inside an outer tx) — atomicity comes from **per-batch undo** (a
  compensating rollback that deletes the batch's own cases/recipes, then deletes a batch doctor only
  when it has no remaining children, so the FK cascade can never reap another batch's or manually
  added rows; a still-referenced batch doctor is left in place). Undo keeps the append-only `Revision`
  audit entries (they record the import happened). 148 Go tests green.

- **P5 — district-managed herb catalog (in progress, branch `feat/p5-district-herbs`)**: the herb
  catalog stays ONE shared province-wide list (forced by cross-district herb filtering), but write
  access widens. A district editor may **add** herbs (stamped with its district) and **edit the ones
  its district created** (`ErrNotOwner` → 403 otherwise); only the central admin edits across
  districts and runs the **merge/alias** tool (mark herb B an alias of A and re-point every
  `Ingredient.HerbID` from B→A). `Herb` gains `CreatedByDistrictID` + `AliasOfID`. A save-time
  **near-duplicate** lookup (`GET /api/herbs/near-duplicates?thaiName=`) nudges against dupes. Routes:
  `POST/PUT /api/herbs` now `RequireAuth` (was admin-only); `POST /api/herbs/:id/merge/:canonicalId`
  and reconcile/delete/list stay admin. Provenance + alias are immutable through Update (no ownership
  hijack). The P1 pending-herb path stays as a fallback. Changes the ownership rule in FR-HERB-1. 156
  Go tests green.

- **P2–P5 frontend admin screens (done, branch `feat/p2-p5-frontend`)**: the P2–P5 backend flows had
  no UI. Four central-admin screens are added to the Next.js staff app, plus role-gated navigation.
  The staff nav now shows the admin links (`review`, `year-locks`, `imports`) only to a
  `central_admin`; a new `RequireAdmin` guard bounces a district editor off an admin URL. Screens:
  **`/staff/review`** (approval queue) lists the pending queue and, for each item, fetches a new
  `GET /api/review/entry/:entityType/:entityId` detail (identity + owning `doctorId` + current vs
  proposed change) so the admin sees what to approve; it approves, rejects with a per-row required
  reason, or approves a whole doctor tree. **`/staff/year-locks`** locks and unlocks a `data_year`
  and shows the "pending queue not empty" refusal inline. **`/staff/imports`** uploads an `.xlsx`,
  runs a dry-run report (counts, skipped, errors), commits through the domain services, and undoes a
  batch behind a confirm. The herb page (**`/staff/herbs`**) gains a dedicated add-form with a
  debounced near-duplicate warning and an admin-only merge panel; the earlier reconcile flow stays.
  New backend: the review detail endpoint (`review` package, central-admin, 163 Go tests green). New
  frontend helper `api.upload` for multipart. Tests: one Playwright spec per screen; the whole e2e
  suite (19 specs) is green. The suite now runs serially (`workers: 1`) because the single shared
  SQLite dev DB makes parallel write-heavy specs flaky. Still frontend-only follow-ups (parked):
  no per-worker e2e DB isolation; the queue diff renders a recipe `{recipe,ingredients}` payload as
  one cell.

## Data model (summary)

Nine records: District, User, Doctor, Herb (shared catalog; P5 adds provenance + alias), Recipe, Case,
Revision (P2 audit log), YearLock (P3 read-only freeze per `data_year`), ImportBatch (P4 bulk-import
group + undo).

```
District ──< Doctor ──< Recipe ──< Case
                          └──< Ingredient >── Herb
Revision (append-only): entity_type + entity_id → who/when/action/after_json
YearLock: data_year (pk) → locked_at/locked_by  (freezes that year's Recipe/Case)
ImportBatch: id → imported_by/at, source_file, row_count, status; Doctor/Recipe/Case carry BatchID
```

- Case links to one Recipe. Patient is anonymous.
- Ingredient uses a decimal amount plus a unit, and links to the shared Herb catalog
  (or a pending-herb name when the herb is not in the catalog yet).
- Doctor and Recipe carry a short `code` for reliable linking on the paper form.
- Doctor is one row per healer (`first_year`); Recipe/Case carry `data_year`.
- Doctor needs consent before it goes public. **P2**: Doctor/Recipe/Case also carry an approval
  workflow (`review_state` + `pending_json`/`pending_delete`/`rejection_reason`); public requires
  `review_state = 'approved'` AND consent. Consent and review are independent gates.
- **P3**: a `data_year` can be locked (frozen read-only) via the `YearLock` table; recipe/case
  writes into a locked year are refused (409), except the admin PDPA-erasure delete.

