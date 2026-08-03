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
- Next step: execute plan v2 (subagent-driven-development).

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

