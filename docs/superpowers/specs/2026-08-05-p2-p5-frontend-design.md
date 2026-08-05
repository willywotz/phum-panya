# P2–P5 Frontend Admin Screens — Design

Date: 2026-08-05 · Branch: `feat/p2-p5-frontend`

## 1. Goal

The P2–P5 backend flows (approval queue, year locking, bulk import, herb
merge/near-duplicate) are merged to `main` but have **no frontend**. This work
adds the missing central-admin screens to the existing Next.js staff shell so
the phases become usable. Scope: the four admin flows only. No public-side or
editor-facing changes beyond role-gated navigation.

## 2. Constraints (from repo rules)

- Talk in ASD-STE100 Simplified Technical English in prose.
- Mandatory TDD: failing test → confirm fail → minimal code → confirm pass.
- API routes stay full English names (already fixed on the backend).
- 15-Factor + Hexagonal on the backend; the one backend addition respects the
  existing package boundaries (lives in `review`).
- Google style guides (TS/HTML-CSS). Clean, minimal code; American English
  names; organized imports.
- End of task: update `CONTEXT.md`, then commit.

## 3. Existing patterns reused

- Pages: `app/(staff)/staff/<name>/page.tsx`, `'use client'`.
- Fetch: `lib/api.ts` (`api.get` / `api.send`, `ApiError`, credentialed
  same-origin, no CSRF header).
- Auth: `lib/auth.tsx` `useMe()` returns `{ id, role, district_id }`;
  `RequireStaff` guards the staff group.
- i18n: bilingual dictionary in `lib/i18n.tsx` (`th` default + `en`), `useT()`,
  `LangToggle`.
- UI: shadcn/ui in `components/ui/*` (button, dialog, alert-dialog, input,
  label, select, table, textarea, checkbox, card).
- CRUD scaffold: `ResourceSpec` + `CrudTable`/`CrudForm`; `rowId`/`rowValue`
  tolerate Go PascalCase vs json-tag casing.
- Tests: Playwright specs in `e2e/*.spec.ts`.

## 4. Shared changes

### 4.1 Role-gated navigation
`app/(staff)/staff/layout.tsx` splits `navLinks` into editor links (current set)
and **admin-only** links (`review`, `year-locks`, `imports`). The nav renders
admin links only when `useMe().role === 'central_admin'`. Herb merge lives on
the existing `/staff/herbs` page, so no extra nav entry for it.

### 4.2 Admin route guard
New `RequireAdmin` wrapper in `lib/auth.tsx` (mirrors `RequireStaff`): when
`me.role !== 'central_admin'` it redirects to `/staff`. Every admin page wraps
its body so a hand-typed URL still bounces a district editor. Non-admins never
depend on client-side hiding alone — the API already enforces the role (403).

### 4.3 Review detail endpoint (backend, TDD)
The queue API returns only `{ entityType, entityId, action, reviewState,
districtId }` — not the entity name or the proposed content. To let the admin
see what they approve, add:

```
GET /api/review/entry/:entityType/:entityId   # central_admin
```

Response:

```json
{
  "entityType": "doctor",
  "entityId": 12,
  "action": "update",
  "identity": "นายสมชาย ...",        // human label: doctor full name, recipe/case title
  "doctorId": 12,                     // owning doctor; for a doctor entity == entityId
  "current":  { ...approved fields... },   // null for a pending create
  "proposed": { ...pending fields...  }     // null for a pending delete
}
```

`doctorId` lets the client group recipes/cases under their doctor and offer
"approve all" (`/api/review/doctor/:doctorId/approve-all`) without a second
lookup.

Implementation notes:
- Lives in the `review` package (`GET` handler + repo method), keeping approval
  concerns in one module (Hexagonal boundary preserved).
- For `update`: `current` = the live row, `proposed` = decoded `pending_json`.
- For `create`: `current` = null, `proposed` = the pending row itself.
- For `delete`: `current` = the live row, `proposed` = null.
- Admin already reads full staff data, so returning full fields adds no new PDPA
  exposure. The endpoint is `central_admin`-only.

### 4.4 Multipart upload helper
`lib/api.ts` gains `api.upload<T>(path, formData)`: same credentialed fetch, no
`Content-Type` header (the browser sets the multipart boundary), same
`ApiError` handling. Needed by the import screen; JSON paths unchanged.

### 4.5 i18n
New keys (th + en) for every label, action, and empty/error state introduced
below. Thai is the default locale.

## 5. Screens

### 5.1 Approval queue — `/staff/review`
- `GET /api/review/queue` → items. Enrich each, then group in the client by
  `districtId`, then by the owning `doctorId` (recipes/cases nest under their
  doctor).
- Enrich each item via `GET /api/review/entry/:type/:id`: show identity, an
  action badge (create / update / delete), and for `update` a proposed-vs-current
  field diff (only changed fields).
- Row actions:
  - **Approve** → `POST /api/review/entry/:type/:id/approve`.
  - **Reject** → `AlertDialog` with a required reason → `POST …/reject`
    `{ "reason": "..." }`.
  - **Approve all** (per doctor) → `POST /api/review/doctor/:doctorId/approve-all`.
- Refetch the queue after each action. Empty state when the queue is clear.

### 5.2 Year locks — `/staff/year-locks`
- `GET /api/year-locks` → table of locked `data_year` values.
- **Lock**: number input (Buddhist year, e.g. 2567) → `POST /api/year-locks`
  `{ "dataYear": 2567 }`. The backend refuses when the pending queue is not
  empty; surface that error message inline.
- **Unlock**: `DELETE /api/year-locks/:dataYear` behind an `AlertDialog`
  confirm.

### 5.3 Bulk import — `/staff/imports`
- File picker (`.xlsx`, field name `file`).
- **Dry run**: `api.upload('/api/imports?dryRun=true', fd)` → render the
  `Report`: counts (`doctorsNew`, `recipesNew`, `casesNew`), a `skipped[]` table
  (`sheet`, `code`, `reason`), and an `errors[]` table (`sheet`, `ref`,
  `message`).
- **Commit**: enabled after a dry run → `dryRun=false` → show the returned
  `batchId`.
- **Undo last batch**: `POST /api/imports/:batchId/undo` behind a confirm, using
  the just-returned `batchId`. (Undo is the documented remedy for duplicate
  case rows.)

### 5.4 Herb merge + near-duplicate — extend `/staff/herbs`
- **Merge panel** (admin-only, mirrors the existing `ReconcilePanel`): select a
  source herb + a canonical herb → confirm → `POST /api/herbs/:id/merge/:canonicalId`.
  Re-points ingredients server-side; refetch the catalog after.
- **Near-duplicate warning**: a dedicated herb add-form (herb-specific, not the
  generic `CrudForm`) that, as the user types `thai_name`, calls
  `GET /api/herbs/near-duplicates?thaiName=…` (debounced) and lists likely
  matches as an advisory warning before save. Save still goes to
  `POST /api/herbs`.

## 6. Error handling

- `ApiError` from `lib/api.ts` carries `status` + parsed `body`. Screens show the
  backend `body.error`/message inline (e.g. year-lock refusal, merge conflict).
- `401` → redirect to `/login` (existing behavior via guards).
- `403` should not occur for admins; if it does (role changed mid-session), show
  a generic "not permitted" and let the guard re-evaluate.

## 7. Testing

- **Backend (Go, TDD):** table test for the review detail endpoint covering
  create / update / delete shapes and the `central_admin` guard. Red → green.
- **Frontend (Playwright), one spec per screen:**
  - `e2e/review-queue.spec.ts` — editor edits → item appears → admin sees diff →
    approve; and a reject-with-reason path.
  - `e2e/year-locks.spec.ts` — lock a year, unlock it; the "pending not empty"
    refusal.
  - `e2e/imports.spec.ts` — upload fixture `.xlsx` → dry-run report → commit →
    undo.
  - `e2e/herb-merge.spec.ts` — near-duplicate warning on add; admin merge
    re-points and the alias disappears from the working view.
- Gate: `go test ./...` and `npx playwright test` green before merge.

## 8. Out of scope

- No public-side or editor CRUD changes beyond nav gating.
- No fix to the parked `SetPhoto` gate gaps or `Merge` self/chain guard (§8 of
  HANDOFF) — separate hardening.
- No new release tag; that is decided after UI + UAT land.

## 9. Deliverables

- `feat/p2-p5-frontend` branch → PR, CI green (go + web type-check + e2e).
- `CONTEXT.md` updated; commit.
- Screens: `/staff/review`, `/staff/year-locks`, `/staff/imports`, extended
  `/staff/herbs`; role-gated nav; `RequireAdmin`; `api.upload`; the review
  detail endpoint.
