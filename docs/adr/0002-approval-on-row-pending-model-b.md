# Approval before publish with an on-row pending model (Model B)

Status: accepted

Context: paid phase P2 (`docs/superpowers/plans/2026-08-04-p2-p5-scope.md`).

## Decision

A district-editor save no longer publishes at once. Every district-editor write
enters a **pending** state and the central admin approves it before the public
sees it (**Model B — gate every change**, not only the first publication). The
system keeps a **full edit history** in an append-only `Revision` table.

The pending state lives **on the row itself**, not in a separate queue table.
Each of `Doctor`, `Recipe`, `Case` gains four columns:

- `review_state` — `pending` | `approved` | `rejected`; the approval status of the
  real columns. New create → `pending`; admin write / approved → `approved`;
  rejected create → `rejected`.
- `pending_json` — the full proposed after-state for an **edit to an already
  approved row**. While set, the real columns stay unchanged and public.
- `pending_delete` — an editor proposed deleting an approved row.
- `rejection_reason` — the reason from the last reject; distinguishes a rejected
  pending edit/delete (reason set) from a still-pending one (reason nil).

Public visibility is gated in **one place** (`internal/publicapi`): a row is
public only if `review_state = 'approved'` **and** the pre-existing consent +
ancestor-chain gate passes. Consent and review are **independent** gates.

- **District-editor writes queue**; **central-admin writes are immediate**
  (auto-approved and logged) — the admin is the approver, and the PDPA
  erasure/unpublish path is an admin action.
- Approve promotes the pending state (apply overlay / hard-delete / flip to
  approved) and appends a `Revision`. Reject returns the change to the editor
  with a reason and is also logged. A bulk "approve all pending for this doctor"
  walks the doctor tree.

## Why

This is public medical content. An unreviewed dosage edit must not reach the
public, so the gate covers **every** change, not just first publication.

The pending state is **on-row** rather than a clean "live-table-always-approved"
queue because a pending **new parent** must own a real, stable id/code from
creation so its children (a Recipe under a not-yet-approved Doctor) can link to
it at once. A separate queue table cannot hand a queued-but-unsaved parent a real
id. The cost is four extra columns and a `review_state = 'approved'` clause on
every public read; both are cheap and centralized.

## Considered options

- **First-publication-only gate** — only a brand-new record needs approval;
  later edits publish instantly. Rejected: an edit to an approved dosage would
  reach the public unreviewed.
- **Separate live/queue tables** — the live table holds only approved rows; a
  parallel queue table holds pending changes. Rejected: a pending new parent has
  no real id for its children to reference, breaking attribution (FR-PUB-2 /
  FR-LINK-1) at creation time.
- **Event-sourced revisions as the source of truth** — rebuild current state by
  folding the revision log. Rejected as over-engineering for one small
  self-hosted app; we keep current state in the row and use `Revision` as an
  append-only audit trail (and the base for P3 point-in-time reconstruction).

## Consequences

- **One extra clause on every public read.** `review_state = 'approved'` is added
  next to each consent gate in `internal/publicapi/public.go`. Any new public
  read path must route through the gated queries — a missed path would leak
  unreviewed content.
- **Revision append happens after the write transaction commits.** `revision.
  Append` opens its own pooled connection; calling it inside an open write
  transaction deadlocks SQLite's single WAL writer (`SQLITE_BUSY`). Both the
  immediate write paths and the review approve/reject paths collect the revision
  and append it post-commit (the same convention as `internal/recipe`).
- **PDPA fields stay private.** `pending_json`, `rejection_reason`, and the
  `Revision` `after_json` are never selected into a public response or an export.
- **Portable GORM only.** The workflow uses plain columns and parameterized
  queries — no SQLite-only feature — so the Postgres option stays a driver swap.
- **P3/P4 reuse the `Revision` trail.** Year-lock point-in-time reconstruction and
  the bulk-import audit build on it; it is built once here.
