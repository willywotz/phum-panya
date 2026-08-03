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
  cards), and a **vendored Thai font** (`@fontsource/noto-sans-thai`) — no CDN, so the single
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
- Next step: merge `feat/p1-launch` to `main`; then P1 acceptance/UAT with the client. The
  styling pass sits on top of `feat/p1-launch` — decide merge order (styling → p1-launch → main,
  or fold together). The dev-compose branch is a small independent add-on on top.

## Data model (summary)

Six records: District, User, Doctor, Herb (shared catalog), Recipe, Case.

```
District ──< Doctor ──< Recipe ──< Case
                          └──< Ingredient >── Herb
```

- Case links to one Recipe. Patient is anonymous.
- Ingredient uses a decimal amount plus a unit, and links to the shared Herb catalog
  (or a pending-herb name when the herb is not in the catalog yet).
- Doctor and Recipe carry a short `code` for reliable linking on the paper form.
- Doctor is one row per healer (`first_year`); Recipe/Case carry `data_year`.
- Doctor needs consent before it goes public. Year locking is a later paid feature.

