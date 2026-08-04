# P2–P5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the four paid phases from `docs/superpowers/plans/2026-08-04-p2-p5-scope.md` — P2 approval-before-publish + edit history, P3 year locking, P4 bulk import, P5 district-managed herb catalog — on top of the shipped `v1.0.0` baseline.

**Architecture:** Follow the existing hexagonal-ish shape: one concrete `*Repo{g *gorm.DB, clk clock.Clock}` per domain package, gin handlers in the same package, wiring in `internal/router/router.go`. New work goes in new packages (`revision`, `review`, `yearlock`, `importer`) or extends `herb`. The public read filter stays in one place (`internal/publicapi/public.go`). All persistence stays portable GORM (no SQLite-only feature); Postgres migration is out of scope.

**Tech Stack:** Go 1.26, gin, GORM + `github.com/glebarez/sqlite` (cgo-free, WAL, FK on), `github.com/xuri/excelize/v2` (already a dep, write-only today), stdlib `testing` + `net/http/httptest`, `github.com/disintegration/imaging` for media.

## Global Constraints

Every task's requirements implicitly include this section. Values copied verbatim from repo rules and the scope doc.

- **TDD, always.** Write the failing test first, run it, confirm it fails, then minimal code, confirm pass, refactor. No exceptions.
- **Branch first.** Never commit multi-task work to `main`. Each phase runs on its own branch: `feat/p2-approval-history`, `feat/p3-year-locking`, `feat/p4-bulk-import`, `feat/p5-district-herbs`.
- **RTK.** Prefix every shell command with `rtk` (e.g. `rtk go test ./...`, `rtk git commit`), including inside `&&` chains.
- **Hexagonal:** new domain logic lives in its own package; handlers are adapters that call the `Repo`. No business rules in handlers beyond auth gating.
- **15-Factor:** config from env only (`APP_*` in `internal/config/config.go`), logs to stdout, stateless process.
- **PDPA:** private fields (`phone`, `consent_*`, `updated_*`, and now `pending_json`, `rejection_reason`, revision `after_json`) must NEVER appear in a public (`publicapi`) response or an export.
- **Portable GORM only.** No SQLite-only SQL. Keep everything a driver swap away from Postgres.
- **Roles** are the strings `"central_admin"` and `"district_editor"`. This plan adds typed constants `model.RoleCentralAdmin` / `model.RoleDistrictEditor` and uses them in all new code.
- **API routes** use full English names, no abbreviations (e.g. `/api/review/queue`, not `/api/rev/q`).
- **Google/Uber-Go style**, American English naming, no plural `xxxList` names, organized imports.
- **When a phase is done:** update `CONTEXT.md` (`## Status` dated bullet + `## Data model (summary)` if the schema changed) and commit it, per repo rules. Write an ADR at funding time for P2's on-row-pending / Model-B design.

## Sequence

Build in dependency order: **P2 → P3 → P4 → P5**. P2 is foundational (its `Revision` trail and `review_state` filter are reused by P3 and P4). P5 is independent but done last, after the schema settles.

---

## File Structure

New and modified files across all four phases.

**P2 — approval + history**
- Modify: `internal/model/model.go` — pending columns on Doctor/Recipe/Case; new `Revision` struct; role constants; `AutoMigrate` list.
- Create: `internal/revision/revision.go` — append-only revision log repo.
- Create: `internal/revision/revision_test.go`
- Create: `internal/review/review.go` — approval-queue domain: list/approve/reject/bulk-approve, promotes/rolls back on-row pending state.
- Create: `internal/review/handler.go` — admin-only HTTP adapter.
- Create: `internal/review/review_test.go`
- Modify: `internal/doctor/doctor.go`, `internal/recipe/recipe.go`, `internal/caserec/case.go` — write path branches on `immediate` (admin) vs queue (editor); repos gain a `*revision.Repo`.
- Modify: `internal/doctor/handler.go`, `internal/recipe/handler.go`, `internal/caserec/handler.go` — compute `immediate` from role, pass it down.
- Modify: `internal/publicapi/public.go` — add `review_state = 'approved'` to every public query, in one shared place.
- Modify: `internal/router/router.go` — build `revision.NewRepo(db)`, thread into domain repos, register `review` routes.

**P3 — year locking**
- Modify: `internal/model/model.go` — `YearLock` struct; `AutoMigrate` list.
- Create: `internal/yearlock/yearlock.go` — lock/unlock/list + `IsLocked(dataYear)`; guards pending-queue-empty precondition.
- Create: `internal/yearlock/handler.go` — admin-only HTTP adapter.
- Create: `internal/yearlock/yearlock_test.go`
- Modify: `internal/recipe/recipe.go`, `internal/caserec/case.go` — write guard: refuse create/update/delete on a locked `data_year`, except the admin PDPA-erasure path.
- Modify: `internal/router/router.go` — wire yearlock into recipe/caserec repos and register routes.

**P4 — bulk import**
- Modify: `internal/model/model.go` — `ImportBatch` struct + `BatchID *uint` tags on Doctor/Recipe/Case; `AutoMigrate` list.
- Create: `internal/importer/template.go` — canonical template column definitions (Sheets A/B/C twin).
- Create: `internal/importer/parse.go` — parse the template into typed rows.
- Create: `internal/importer/importer.go` — dry-run validate → report → commit-in-one-transaction via domain services; per-batch undo.
- Create: `internal/importer/handler.go` — admin-only HTTP adapter (multipart upload, `?dryRun=true`).
- Create: `internal/importer/*_test.go`
- Modify: `internal/router/router.go` — register importer routes with the domain repos it reuses.

**P5 — district-managed herb catalog**
- Modify: `internal/model/model.go` — `Herb` gains `CreatedByDistrictID *uint`, `AliasOfID *uint`; `AutoMigrate` unchanged (same struct).
- Modify: `internal/herb/herb.go` — widen write port: editor may create + edit own; central-admin merge/alias; near-duplicate check at save.
- Modify: `internal/herb/handler.go` — replace blanket `RequireRole("central_admin")` with per-action gating.
- Create/Modify: `internal/herb/herb_test.go`
- Modify: `internal/router/router.go` — herb repo gains the merge dependency (re-point ingredients).

---

# Phase P2 — Approval before publish + edit history

**Branch:** `feat/p2-approval-history` (create before Task P2.1).

### State model (the design tasks below implement)

Four new per-entity columns drive a Model-B, on-row, per-record approval workflow:

| Column | Meaning |
|---|---|
| `ReviewState string` | Approval status of the **real columns**: `pending` \| `approved` \| `rejected`. A create starts `pending`; admin/approved rows are `approved`; a rejected create is `rejected`. |
| `PendingJSON *string` | Full proposed after-state (entity as JSON) for an **edit to an already-approved row**. `nil` when there is no pending edit. Real columns stay unchanged while this is set. |
| `PendingDelete bool` | An editor proposed deleting an approved row. |
| `RejectionReason *string` | Reason from the last reject; distinguishes a *rejected* pending edit/delete (reason set) from a *still-pending* one (reason nil). |

**Public visibility (one place, `publicapi`):** a row is public only if `review_state = 'approved'` **and** its consent + ancestor-chain gate already passes. This hides pending/rejected creates, and keeps showing an approved row that merely has a pending edit overlay.

**Queue membership (needs admin action):**
```
review_state = 'pending'
OR (pending_json IS NOT NULL AND rejection_reason IS NULL)
OR (pending_delete = 1 AND rejection_reason IS NULL)
```

**Write path by actor:**
- `district_editor` (`immediate = false`): create → insert real row `pending`; edit → keep real columns, marshal proposal into `pending_json`, clear `rejection_reason`; delete → set `pending_delete = true`, clear `rejection_reason`.
- `central_admin` (`immediate = true`): apply directly, `review_state = 'approved'`, and append a `Revision` row. No queueing (the admin is the approver).

**Approve/Reject (admin, in `review` package):**
- Approve create: `review_state = 'approved'`; append revision `create`.
- Approve edit: unmarshal `pending_json` → overwrite real columns; clear `pending_json` + `rejection_reason`; append revision `update`.
- Approve delete: hard-delete via the domain repo (cascade); append revision `delete` (tombstone snapshot).
- Reject create: `review_state = 'rejected'` + reason; append revision `reject`.
- Reject edit: keep `pending_json`, set reason (real columns stay `approved`); append revision `reject`.
- Reject delete: clear `pending_delete`, set reason; append revision `reject`.

### Contracts (signatures other tasks rely on)

```go
// internal/model/model.go
const (
    RoleCentralAdmin   = "central_admin"
    RoleDistrictEditor = "district_editor"

    ReviewPending  = "pending"
    ReviewApproved = "approved"
    ReviewRejected = "rejected"

    ActionCreate = "create"
    ActionUpdate = "update"
    ActionDelete = "delete"
    ActionReject = "reject"
)

type Revision struct {
    ID         uint      `gorm:"primaryKey"`
    EntityType string    `gorm:"not null;index:idx_rev_entity"`
    EntityID   uint      `gorm:"not null;index:idx_rev_entity"`
    ChangedBy  uint      `gorm:"not null"`
    ChangedAt  time.Time `gorm:"not null"`
    Action     string    `gorm:"not null"`
    AfterJSON  string    `gorm:"not null"`
}

// internal/revision/revision.go
func NewRepo(g *gorm.DB, clk clock.Clock) *Repo
func (r *Repo) Append(entityType string, entityID, changedBy uint, action string, after any) error
func (r *Repo) List(entityType string, entityID uint) ([]model.Revision, error)

// internal/review/review.go
type Item struct {
    EntityType   string `json:"entityType"`
    EntityID     uint   `json:"entityId"`
    Action       string `json:"action"`       // create | update | delete
    ReviewState  string `json:"reviewState"`
    DistrictID   uint   `json:"districtId"`
    RejectedWhy  string `json:"rejectedWhy,omitempty"`
}
func NewRepo(g *gorm.DB, rev *revision.Repo) *Repo
func (r *Repo) Queue(districtID *uint) ([]Item, error)   // nil districtID = all districts
func (r *Repo) Approve(entityType string, entityID, actorID uint) error
func (r *Repo) Reject(entityType string, entityID, actorID uint, reason string) error
func (r *Repo) ApproveDoctorTree(doctorID, actorID uint) (int, error) // bulk button

// domain repo write signatures gain an immediate flag (admin=true)
func (r *doctor.Repo) Create(d *model.Doctor, actorID uint, immediate bool) error
func (r *doctor.Repo) Update(d *model.Doctor, actorID uint, immediate bool) error
func (r *doctor.Repo) Delete(id, actorID uint, immediate bool) error
// recipe/caserec: same shape (recipe Create/Update also take []model.Ingredient).
```

---

### Task P2.1: Schema — pending columns, Revision model, role constants

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/model_test.go` (create)

**Interfaces:**
- Produces: the constants and `Revision` struct in Contracts above; new columns `ReviewState`, `PendingJSON`, `PendingDelete`, `RejectionReason` on `Doctor`, `Recipe`, `Case`.

- [ ] **Step 1: Write the failing test**

```go
// internal/model/model_test.go
package model_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestAutoMigrateCreatesRevisionAndPendingColumns(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Revision table round-trips.
	rev := model.Revision{EntityType: "doctor", EntityID: 1, ChangedBy: 2, Action: model.ActionCreate, AfterJSON: "{}"}
	if err := g.Create(&rev).Error; err != nil {
		t.Fatalf("insert revision: %v", err)
	}

	// A new doctor defaults to review_state = pending via app code; column exists and is writable.
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, ReviewState: model.ReviewPending}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("insert doctor: %v", err)
	}
	var got model.Doctor
	if err := g.First(&got, d.ID).Error; err != nil {
		t.Fatalf("read doctor: %v", err)
	}
	if got.ReviewState != model.ReviewPending {
		t.Fatalf("review_state = %q, want %q", got.ReviewState, model.ReviewPending)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/model/ -run TestAutoMigrateCreatesRevisionAndPendingColumns -v`
Expected: FAIL — `model.Revision` / `model.ReviewPending` undefined.

- [ ] **Step 3: Add constants, Revision struct, columns, migration**

In `internal/model/model.go`, add the `const` block and `Revision` struct from Contracts. Add these fields to `Doctor`, `Recipe`, and `Case` (place after each struct's `UpdatedAt`):

```go
	ReviewState     string  `gorm:"not null;default:approved;index"`
	PendingJSON     *string
	PendingDelete   bool `gorm:"not null;default:false"`
	RejectionReason *string
```

> `default:approved` keeps every pre-existing `v1.0.0` row public after `AutoMigrate` backfills the column (they were already public). New creates set `ReviewPending` explicitly in app code (Task P2.4–P2.6).

Append `&Revision{}` to `AutoMigrate`:

```go
func AutoMigrate(g *gorm.DB) error {
	return g.AutoMigrate(
		&District{}, &User{}, &Session{}, &Doctor{},
		&Herb{}, &Recipe{}, &Ingredient{}, &Case{},
		&Revision{},
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/model/ -run TestAutoMigrateCreatesRevisionAndPendingColumns -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/model/model.go internal/model/model_test.go && rtk git commit -m "feat(p2): add pending columns, Revision model, role/state constants"
```

---

### Task P2.2: `revision` package — append-only history log

**Files:**
- Create: `internal/revision/revision.go`
- Test: `internal/revision/revision_test.go`

**Interfaces:**
- Consumes: `model.Revision`, `clock.Clock`.
- Produces: `revision.NewRepo`, `(*Repo).Append`, `(*Repo).List` (see Contracts).

- [ ] **Step 1: Write the failing test**

```go
// internal/revision/revision_test.go
package revision_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

func TestAppendThenListInOrder(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := revision.NewRepo(g, clock.Real{})

	type snap struct {
		Name string
	}
	if err := repo.Append("doctor", 7, 1, model.ActionCreate, snap{"before"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := repo.Append("doctor", 7, 1, model.ActionUpdate, snap{"after"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	got, err := repo.List("doctor", 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Action != model.ActionCreate || got[1].Action != model.ActionUpdate {
		t.Fatalf("wrong order: %q, %q", got[0].Action, got[1].Action)
	}
	if got[1].AfterJSON != `{"Name":"after"}` {
		t.Fatalf("after_json = %q", got[1].AfterJSON)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/revision/ -v`
Expected: FAIL — package `revision` does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/revision/revision.go
package revision

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// Repo appends and reads the immutable edit history (FR-AUDIT-1, extended).
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// Append writes one immutable revision row. after is marshalled to JSON.
func (r *Repo) Append(entityType string, entityID, changedBy uint, action string, after any) error {
	blob, err := json.Marshal(after)
	if err != nil {
		return err
	}
	rev := model.Revision{
		EntityType: entityType,
		EntityID:   entityID,
		ChangedBy:  changedBy,
		ChangedAt:  r.clk.Now(),
		Action:     action,
		AfterJSON:  string(blob),
	}
	return r.g.Create(&rev).Error
}

// List returns the history for one entity, oldest first.
func (r *Repo) List(entityType string, entityID uint) ([]model.Revision, error) {
	var out []model.Revision
	err := r.g.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("id ASC").Find(&out).Error
	return out, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/revision/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/revision/ && rtk git commit -m "feat(p2): add revision append-only history log"
```

---

### Task P2.3: Doctor write path — queue for editor, immediate for admin

**Files:**
- Modify: `internal/doctor/doctor.go`
- Modify: `internal/doctor/handler.go`
- Test: `internal/doctor/doctor_test.go` (extend)

**Interfaces:**
- Consumes: `revision.Repo`, `model` constants.
- Produces: `doctor.Repo` with a `rev *revision.Repo` field; `NewRepo(g, clk, rev)`; write methods take `immediate bool` (see Contracts).

- [ ] **Step 1: Write the failing test**

Add to `internal/doctor/doctor_test.go`. The existing `newDoctorAPI` helper already seeds an admin + an editor and provides `doAsAdmin`/`doAsEditor`; extend its `doctor.NewRepo(...)` call to pass `revision.NewRepo(g, clock.Real{})`.

```go
func TestEditorCreateGoesPendingAdminIsImmediate(t *testing.T) {
	env := newDoctorAPI(t)

	// Editor create -> pending, hidden from public read filter (verified in P2 public task).
	body := `{"code":"D9","fullName":"เจ้าของ","specialty":"ยาต้ม","status":"active","firstYear":2568,"districtId":1}`
	rec := env.doAsEditor(t, "POST", "/api/doctors", body)
	if rec.Code != 201 {
		t.Fatalf("editor create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created model.Doctor
	env.g.Where("code = ?", "D9").First(&created)
	if created.ReviewState != model.ReviewPending {
		t.Fatalf("editor create review_state = %q, want pending", created.ReviewState)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", created.ID).Count(&revCount)
	if revCount != 0 {
		t.Fatalf("editor create should not append a revision yet, got %d", revCount)
	}

	// Admin create -> approved immediately + one revision.
	body2 := `{"code":"D10","fullName":"แอดมิน","specialty":"ยาต้ม","status":"active","firstYear":2568,"districtId":1}`
	rec2 := env.doAsAdmin(t, "POST", "/api/doctors", body2)
	if rec2.Code != 201 {
		t.Fatalf("admin create status = %d", rec2.Code)
	}
	var adminDoc model.Doctor
	env.g.Where("code = ?", "D10").First(&adminDoc)
	if adminDoc.ReviewState != model.ReviewApproved {
		t.Fatalf("admin create review_state = %q, want approved", adminDoc.ReviewState)
	}
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", adminDoc.ID).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("admin create revisions = %d, want 1", revCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/doctor/ -run TestEditorCreateGoesPendingAdminIsImmediate -v`
Expected: FAIL — `NewRepo` arity / `immediate` param mismatch, or `ReviewState` unset.

- [ ] **Step 3: Update the repo and handler**

In `internal/doctor/doctor.go` add the `rev` field and branch:

```go
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
	rev *revision.Repo
}

func NewRepo(g *gorm.DB, clk clock.Clock, rev *revision.Repo) *Repo {
	return &Repo{g: g, clk: clk, rev: rev}
}

// Create inserts a doctor. Editor creates enter the pending queue; admin creates
// publish immediately and are logged.
func (r *Repo) Create(d *model.Doctor, actorID uint, immediate bool) error {
	d.UpdatedBy = &actorID
	d.UpdatedAt = r.clk.Now()
	if immediate {
		d.ReviewState = model.ReviewApproved
	} else {
		d.ReviewState = model.ReviewPending
	}
	if err := r.g.Create(d).Error; err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("doctor", d.ID, actorID, model.ActionCreate, d)
	}
	return nil
}

// Update: admin writes real columns immediately; editor writes stash the proposal
// in pending_json and leave the approved columns visible.
func (r *Repo) Update(d *model.Doctor, actorID uint, immediate bool) error {
	var existing model.Doctor
	if err := r.g.First(&existing, d.ID).Error; err != nil {
		return err
	}
	d.Photo = existing.Photo // photo changes go through SetPhoto only

	if immediate {
		d.UpdatedBy = &actorID
		d.UpdatedAt = r.clk.Now()
		d.ReviewState = model.ReviewApproved
		d.PendingJSON = nil
		d.RejectionReason = nil
		if err := r.g.Save(d).Error; err != nil {
			return err
		}
		return r.rev.Append("doctor", d.ID, actorID, model.ActionUpdate, d)
	}

	// Editor edit: keep approved columns, record the proposal.
	proposal := *d
	proposal.ReviewState = existing.ReviewState
	blob, err := json.Marshal(&proposal)
	if err != nil {
		return err
	}
	overlay := string(blob)
	return r.g.Model(&model.Doctor{}).Where("id = ?", d.ID).
		Updates(map[string]any{
			"pending_json":     overlay,
			"pending_delete":   false,
			"rejection_reason": nil,
			"updated_by":       actorID,
			"updated_at":       r.clk.Now(),
		}).Error
}

// Delete: admin deletes now (+revision); editor delete is queued as pending_delete.
func (r *Repo) Delete(id, actorID uint, immediate bool) error {
	if immediate {
		var existing model.Doctor
		if err := r.g.First(&existing, id).Error; err != nil {
			return err
		}
		res := r.g.Delete(&model.Doctor{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.rev.Append("doctor", id, actorID, model.ActionDelete, existing)
	}
	res := r.g.Model(&model.Doctor{}).Where("id = ?", id).
		Updates(map[string]any{"pending_delete": true, "rejection_reason": nil})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
```

Add `"encoding/json"` and the `revision` import. In `internal/doctor/handler.go`, at each write handler compute `immediate` and pass it:

```go
user, _ := auth.UserFrom(c)
immediate := user.Role == model.RoleCentralAdmin
// create:
if err := repo.Create(&d, user.ID, immediate); err != nil { ... }
// update:
if err := repo.Update(&d, user.ID, immediate); err != nil { ... }
// delete:
if err := repo.Delete(uint(id), user.ID, immediate); err != nil { ... }
```

Keep the existing `CanWriteDistrict` checks unchanged — they run before this.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/doctor/ -run TestEditorCreateGoesPendingAdminIsImmediate -v`
Expected: PASS. Also run `rtk go test ./internal/doctor/ -v` to confirm no regression (fix the `NewRepo` call in the test helper if the full suite fails to compile).

- [ ] **Step 5: Commit**

```bash
rtk git add internal/doctor/ && rtk git commit -m "feat(p2): branch doctor write path on role (editor queues, admin immediate)"
```

---

### Task P2.4: Recipe write path — queue vs immediate (composite payload)

**Files:**
- Modify: `internal/recipe/recipe.go`
- Modify: `internal/recipe/handler.go`
- Test: `internal/recipe/recipe_test.go` (extend)

**Interfaces:**
- Consumes: `revision.Repo`, `model` constants, `db.Tx`.
- Produces: `recipe.NewRepo(g, clk, rev)`; `Create(rec, ings, actorID, immediate)`, `Update(rec, ings, actorID, immediate)`, `Delete(id, actorID, immediate)`. A recipe's `pending_json` stores the composite `recipePayload{Recipe, Ingredients}`.

- [ ] **Step 1: Write the failing test**

```go
func TestRecipeEditorEditStashesCompositeProposal(t *testing.T) {
	env := newRecipeAPI(t) // seeds an approved doctor+recipe owned by editor's district

	// Editor edits an approved recipe: real columns stay, pending_json holds proposal.
	body := `{"code":"R1","name":"ยาแก้ไอ (แก้ไข)","doctorCode":"D1","indication":"ไอ","preparation":"ต้ม","usage":"ดื่ม","dataYear":2568,"ingredients":[{"herbId":1,"amount":"10","unit":"g"}]}`
	rec := env.doAsEditor(t, "PUT", "/api/recipes/1", body)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got model.Recipe
	env.g.First(&got, 1)
	if got.ReviewState != model.ReviewApproved {
		t.Fatalf("edit must keep review_state approved, got %q", got.ReviewState)
	}
	if got.PendingJSON == nil {
		t.Fatalf("edit must set pending_json")
	}
	if got.Name == "ยาแก้ไอ (แก้ไข)" {
		t.Fatalf("real columns must not change on an editor edit")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/recipe/ -run TestRecipeEditorEditStashesCompositeProposal -v`
Expected: FAIL — signature mismatch / `PendingJSON` nil.

- [ ] **Step 3: Update the repo and handler**

In `internal/recipe/recipe.go`:

```go
type recipePayload struct {
	Recipe      model.Recipe       `json:"recipe"`
	Ingredients []model.Ingredient `json:"ingredients"`
}

type Repo struct {
	g   *gorm.DB
	clk clock.Clock
	rev *revision.Repo
}

func NewRepo(g *gorm.DB, clk clock.Clock, rev *revision.Repo) *Repo {
	return &Repo{g: g, clk: clk, rev: rev}
}

func (r *Repo) Create(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error {
	if immediate {
		rec.ReviewState = model.ReviewApproved
	} else {
		rec.ReviewState = model.ReviewPending
	}
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		rec.UpdatedBy = &actorID
		rec.UpdatedAt = r.clk.Now()
		if err := tx.Create(rec).Error; err != nil {
			return err
		}
		return createIngredients(tx, rec.ID, ings)
	})
	if err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("recipe", rec.ID, actorID, model.ActionCreate, recipePayload{*rec, ings})
	}
	return nil
}

func (r *Repo) Update(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error {
	var existing model.Recipe
	if err := r.g.First(&existing, rec.ID).Error; err != nil {
		return err
	}
	rec.Photo = existing.Photo

	if immediate {
		rec.ReviewState = model.ReviewApproved
		rec.PendingJSON = nil
		rec.RejectionReason = nil
		err := db.Tx(r.g, func(tx *gorm.DB) error {
			rec.UpdatedBy = &actorID
			rec.UpdatedAt = r.clk.Now()
			if err := tx.Save(rec).Error; err != nil {
				return err
			}
			if err := tx.Where("recipe_id = ?", rec.ID).Delete(&model.Ingredient{}).Error; err != nil {
				return err
			}
			return createIngredients(tx, rec.ID, ings)
		})
		if err != nil {
			return err
		}
		return r.rev.Append("recipe", rec.ID, actorID, model.ActionUpdate, recipePayload{*rec, ings})
	}

	// Editor edit: stash composite proposal, leave approved rows visible.
	proposal := recipePayload{Recipe: *rec, Ingredients: ings}
	blob, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	overlay := string(blob)
	return r.g.Model(&model.Recipe{}).Where("id = ?", rec.ID).
		Updates(map[string]any{
			"pending_json":     overlay,
			"pending_delete":   false,
			"rejection_reason": nil,
			"updated_by":       actorID,
			"updated_at":       r.clk.Now(),
		}).Error
}

func (r *Repo) Delete(id, actorID uint, immediate bool) error {
	if immediate {
		var existing model.Recipe
		if err := r.g.First(&existing, id).Error; err != nil {
			return err
		}
		res := r.g.Delete(&model.Recipe{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.rev.Append("recipe", id, actorID, model.ActionDelete, existing)
	}
	res := r.g.Model(&model.Recipe{}).Where("id = ?", id).
		Updates(map[string]any{"pending_delete": true, "rejection_reason": nil})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
```

Add `"encoding/json"` and `revision` imports. Update the three write handlers in `internal/recipe/handler.go` to compute `immediate := user.Role == model.RoleCentralAdmin` and pass it (same as doctor).

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/recipe/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/recipe/ && rtk git commit -m "feat(p2): branch recipe write path on role with composite pending payload"
```

---

### Task P2.5: Case write path — queue vs immediate

**Files:**
- Modify: `internal/caserec/case.go`
- Modify: `internal/caserec/handler.go`
- Test: `internal/caserec/case_test.go` (extend)

**Interfaces:**
- Consumes: `revision.Repo`, `model` constants.
- Produces: `caserec.NewRepo(g, clk, rev)`; `Create(c, actorID, immediate)`, `Update(c, actorID, immediate)`, `Delete(id, actorID, immediate)`.

- [ ] **Step 1: Write the failing test**

```go
func TestCaseAdminDeleteIsImmediateAndLogged(t *testing.T) {
	env := newCaseAPI(t) // seeds approved doctor+recipe+case

	rec := env.doAsAdmin(t, "DELETE", "/api/cases/1", "")
	if rec.Code != 200 && rec.Code != 204 {
		t.Fatalf("status = %d", rec.Code)
	}
	var count int64
	env.g.Model(&model.Case{}).Where("id = ?", 1).Count(&count)
	if count != 0 {
		t.Fatalf("admin delete must remove the row")
	}
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ? AND action = ?", "case", 1, model.ActionDelete).Count(&count)
	if count != 1 {
		t.Fatalf("admin delete must append a delete revision, got %d", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/caserec/ -run TestCaseAdminDeleteIsImmediateAndLogged -v`
Expected: FAIL — signature mismatch / no revision.

- [ ] **Step 3: Write the implementation**

Mirror the doctor pattern from Task P2.3 in `internal/caserec/case.go` (no photo-preserve subtlety differs; keep the existing `c.Photo = existing.Photo` line in the immediate-update branch). Add the `rev` field, `NewRepo(g, clk, rev)`, and `immediate bool` on `Create`/`Update`/`Delete` with: immediate → set `ReviewApproved`, apply, `r.rev.Append("case", ...)`; editor create → `ReviewPending`; editor edit → `pending_json` overlay of the `model.Case`; editor delete → `pending_delete = true`. Update `internal/caserec/handler.go` to compute and pass `immediate`.

```go
func (r *Repo) Create(c *model.Case, actorID uint, immediate bool) error {
	c.UpdatedBy = &actorID
	c.UpdatedAt = r.clk.Now()
	if immediate {
		c.ReviewState = model.ReviewApproved
	} else {
		c.ReviewState = model.ReviewPending
	}
	if err := r.g.Create(c).Error; err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("case", c.ID, actorID, model.ActionCreate, c)
	}
	return nil
}
```

(Write `Update` and `Delete` the same way as doctor Task P2.3, substituting `model.Case` and the `"case"` entity type; keep `c.Photo = existing.Photo` in the immediate `Update` branch.)

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/caserec/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/caserec/ && rtk git commit -m "feat(p2): branch case write path on role"
```

---

### Task P2.6: Public read filter — require `review_state = approved`

**Files:**
- Modify: `internal/publicapi/public.go`
- Test: `internal/publicapi/public_test.go` (extend)

**Interfaces:**
- Consumes: the `review_state` column.
- Produces: public queries hide any row not `approved`, in one shared place.

- [ ] **Step 1: Write the failing test**

Extend `seedDoctor` usage or add a helper that seeds a `pending` doctor. Then:

```go
func TestPublicHidesPendingDoctorsAndRecipes(t *testing.T) {
	env := newPublicAPI(t)
	// approved + consented doctor "A" with recipe "recipe-A"; pending doctor "P".
	env.seedDoctorState(t, "A", true, model.ReviewApproved, "recipe-A")
	env.seedDoctorState(t, "P", true, model.ReviewPending, "recipe-P")

	body := env.get(t, "/api/public/doctors").Body.String()
	if !strings.Contains(body, "A") {
		t.Fatalf("approved doctor A must be visible")
	}
	if strings.Contains(body, "\"P\"") {
		t.Fatalf("pending doctor P must be hidden")
	}

	rbody := env.get(t, "/api/public/recipes").Body.String()
	if strings.Contains(rbody, "recipe-P") {
		t.Fatalf("recipe of a pending doctor must be hidden")
	}
}
```

(Add a `seedDoctorState(name string, consented bool, state string, recipeName string)` helper alongside the existing `seedDoctor`, setting `ReviewState: state` on the doctor and its recipe.)

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/publicapi/ -run TestPublicHidesPendingDoctorsAndRecipes -v`
Expected: FAIL — pending rows leak into public responses.

- [ ] **Step 3: Add the filter in one place**

In `internal/publicapi/public.go`, add `review_state = 'approved'` next to every existing consent gate. Doctors:

```go
q := r.g.Table("doctors").Select(doctorColumns).
	Where("consent_obtained = ? AND review_state = ?", true, model.ReviewApproved)
```

`GetDoctor`:

```go
Where("id = ? AND consent_obtained = ? AND review_state = ?", id, true, model.ReviewApproved)
```

`recipeQuery()` (the shared base for all recipe reads) — add the recipe's own state plus keep the doctor consent join:

```go
func (r *Repo) recipeQuery() *gorm.DB {
	return r.g.Table("recipes").
		Select(recipeColumns).
		Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
		Joins("JOIN districts ON districts.id = doctors.district_id").
		Where("doctors.consent_obtained = ? AND doctors.review_state = ? AND recipes.review_state = ?",
			true, model.ReviewApproved, model.ReviewApproved)
}
```

`ListCasesByRecipe` — add case + recipe + doctor state to the existing join chain:

```go
Where("cases.recipe_id = ? AND doctors.consent_obtained = ? AND doctors.review_state = ? AND recipes.review_state = ? AND cases.review_state = ?",
	recipeID, true, model.ReviewApproved, model.ReviewApproved, model.ReviewApproved)
```

Import `phum-panya/internal/model`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/publicapi/ -v`
Expected: PASS (including the existing `TestPublicHidesUnconsentedAndPrivateFields`).

- [ ] **Step 5: Commit**

```bash
rtk git add internal/publicapi/ && rtk git commit -m "feat(p2): gate public reads on review_state = approved"
```

---

### Task P2.7: `review` package — queue, approve, reject, bulk

**Files:**
- Create: `internal/review/review.go`
- Test: `internal/review/review_test.go`

**Interfaces:**
- Consumes: `revision.Repo`, `model`, `db.Tx`.
- Produces: `review.NewRepo(g, rev)`, `Queue`, `Approve`, `Reject`, `ApproveDoctorTree` (see Contracts).

- [ ] **Step 1: Write the failing test**

```go
// internal/review/review_test.go
package review_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/review"
	"phum-panya/internal/revision"
)

func setup(t *testing.T) (*review.Repo, *db.DB) { // returns repo + *gorm.DB
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "rev.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return review.NewRepo(g, revision.NewRepo(g, clock.Real{})), g
}

func TestApproveCreatePromotesAndLogs(t *testing.T) {
	repo, g := setup(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: 1, ReviewState: model.ReviewPending}
	g.Create(&d)

	if err := repo.Approve("doctor", d.ID, 99); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.ReviewState != model.ReviewApproved {
		t.Fatalf("state = %q, want approved", got.ReviewState)
	}
	var revs int64
	g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ? AND action = ?", "doctor", d.ID, model.ActionCreate).Count(&revs)
	if revs != 1 {
		t.Fatalf("revisions = %d, want 1", revs)
	}
}

func TestApproveEditAppliesOverlay(t *testing.T) {
	repo, g := setup(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "old", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: 1, ReviewState: model.ReviewApproved}
	g.Create(&d)
	overlay := `{"ID":` + itoa(d.ID) + `,"Code":"D1","FullName":"new","Specialty":"y","Status":"active","FirstYear":2568,"DistrictID":1,"ReviewState":"approved"}`
	g.Model(&model.Doctor{}).Where("id = ?", d.ID).Update("pending_json", overlay)

	if err := repo.Approve("doctor", d.ID, 99); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.FullName != "new" {
		t.Fatalf("overlay not applied: FullName = %q", got.FullName)
	}
	if got.PendingJSON != nil {
		t.Fatalf("pending_json must be cleared after approve")
	}
}

func TestRejectCreateSetsReasonKeepsHidden(t *testing.T) {
	repo, g := setup(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: 1, ReviewState: model.ReviewPending}
	g.Create(&d)

	if err := repo.Reject("doctor", d.ID, 99, "รูปไม่ชัด"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.ReviewState != model.ReviewRejected || got.RejectionReason == nil || *got.RejectionReason != "รูปไม่ชัด" {
		t.Fatalf("reject state/reason wrong: %q / %v", got.ReviewState, got.RejectionReason)
	}
}
```

(Add a tiny local `itoa` helper, or format the overlay with `fmt.Sprintf`. If `db.Open` returns `*gorm.DB` rather than a `*db.DB` wrapper, adjust `setup`'s return type to `*gorm.DB` — match the actual signature in `internal/db/db.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/review/ -v`
Expected: FAIL — package `review` does not exist.

- [ ] **Step 3: Write the implementation**

```go
// internal/review/review.go
package review

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

type Repo struct {
	g   *gorm.DB
	rev *revision.Repo
}

func NewRepo(g *gorm.DB, rev *revision.Repo) *Repo {
	return &Repo{g: g, rev: rev}
}

type Item struct {
	EntityType  string `json:"entityType"`
	EntityID    uint   `json:"entityId"`
	Action      string `json:"action"`
	ReviewState string `json:"reviewState"`
	DistrictID  uint   `json:"districtId"`
	RejectedWhy string `json:"rejectedWhy,omitempty"`
}

var errUnknownEntity = errors.New("review: unknown entity type")

// Queue lists everything awaiting a decision. A nil districtID returns all districts
// (central admin); a non-nil value scopes to one district.
func (r *Repo) Queue(districtID *uint) ([]Item, error) {
	var out []Item

	// Doctors: pending create, pending edit, or pending delete.
	dq := r.g.Table("doctors").
		Select("'doctor' AS entity_type, id AS entity_id, district_id, review_state, pending_json, pending_delete, rejection_reason").
		Where("review_state = ? OR (pending_json IS NOT NULL AND rejection_reason IS NULL) OR (pending_delete = ? AND rejection_reason IS NULL)",
			model.ReviewPending, true)
	if districtID != nil {
		dq = dq.Where("district_id = ?", *districtID)
	}
	if err := scanItems(dq, &out); err != nil {
		return nil, err
	}

	// Recipes and cases resolve their district through the doctor join.
	if err := r.scanChild("recipes", "recipes.doctor_id = doctors.id", districtID, &out); err != nil {
		return nil, err
	}
	if err := r.scanChild("cases", "cases.recipe_id = recipes.id", districtID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Approve promotes a pending create/edit/delete to the live public state and logs it.
func (r *Repo) Approve(entityType string, entityID, actorID uint) error {
	return db.Tx(r.g, func(tx *gorm.DB) error {
		switch entityType {
		case "doctor":
			return approveDoctor(tx, r.rev, entityID, actorID)
		case "recipe":
			return approveRecipe(tx, r.rev, entityID, actorID)
		case "case":
			return approveCase(tx, r.rev, entityID, actorID)
		default:
			return errUnknownEntity
		}
	})
}

// Reject returns a change to the editor with a reason and logs the reject event.
func (r *Repo) Reject(entityType string, entityID, actorID uint, reason string) error {
	table, ok := map[string]string{"doctor": "doctors", "recipe": "recipes", "case": "cases"}[entityType]
	if !ok {
		return errUnknownEntity
	}
	return db.Tx(r.g, func(tx *gorm.DB) error {
		var state string
		var pendingJSON *string
		var pendingDelete bool
		row := tx.Table(table).Select("review_state, pending_json, pending_delete").Where("id = ?", entityID).Row()
		if err := row.Scan(&state, &pendingJSON, &pendingDelete); err != nil {
			return err
		}
		updates := map[string]any{"rejection_reason": reason}
		switch {
		case state == model.ReviewPending:
			updates["review_state"] = model.ReviewRejected // rejected create
		case pendingDelete:
			updates["pending_delete"] = false // rejected delete clears the flag
		}
		// A rejected edit keeps its pending_json untouched.
		if err := tx.Table(table).Where("id = ?", entityID).Updates(updates).Error; err != nil {
			return err
		}
		return r.rev.Append(entityType, entityID, actorID, model.ActionReject, updates)
	})
}

// ApproveDoctorTree approves the doctor and every pending recipe/case beneath it
// (the admin "approve all pending for this doctor" bulk button). Returns the count.
func (r *Repo) ApproveDoctorTree(doctorID, actorID uint) (int, error) {
	n := 0
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		if err := approveDoctor(tx, r.rev, doctorID, actorID); err != nil && !errors.Is(err, errNotPending) {
			return err
		} else if err == nil {
			n++
		}
		var recipeIDs []uint
		tx.Table("recipes").Where("doctor_id = ?", doctorID).Pluck("id", &recipeIDs)
		for _, rid := range recipeIDs {
			if err := approveRecipe(tx, r.rev, rid, actorID); err == nil {
				n++
			} else if !errors.Is(err, errNotPending) {
				return err
			}
			var caseIDs []uint
			tx.Table("cases").Where("recipe_id = ?", rid).Pluck("id", &caseIDs)
			for _, cid := range caseIDs {
				if err := approveCase(tx, r.rev, cid, actorID); err == nil {
					n++
				} else if !errors.Is(err, errNotPending) {
					return err
				}
			}
		}
		return nil
	})
	return n, err
}

var errNotPending = errors.New("review: nothing pending")

func approveDoctor(tx *gorm.DB, rev *revision.Repo, id, actorID uint) error {
	var d model.Doctor
	if err := tx.First(&d, id).Error; err != nil {
		return err
	}
	switch {
	case d.PendingDelete:
		if err := tx.Delete(&model.Doctor{}, id).Error; err != nil {
			return err
		}
		return rev.Append("doctor", id, actorID, model.ActionDelete, d)
	case d.PendingJSON != nil:
		var overlay model.Doctor
		if err := json.Unmarshal([]byte(*d.PendingJSON), &overlay); err != nil {
			return err
		}
		overlay.ID = id
		overlay.Photo = d.Photo
		overlay.ReviewState = model.ReviewApproved
		overlay.PendingJSON = nil
		overlay.RejectionReason = nil
		if err := tx.Save(&overlay).Error; err != nil {
			return err
		}
		return rev.Append("doctor", id, actorID, model.ActionUpdate, overlay)
	case d.ReviewState == model.ReviewPending:
		if err := tx.Model(&model.Doctor{}).Where("id = ?", id).
			Updates(map[string]any{"review_state": model.ReviewApproved, "rejection_reason": nil}).Error; err != nil {
			return err
		}
		return rev.Append("doctor", id, actorID, model.ActionCreate, d)
	default:
		return errNotPending
	}
}

// approveRecipe and approveCase follow the same three-branch shape. The recipe
// overlay is a recipePayload{Recipe, Ingredients}; on approve, replace the recipe
// row and delete-then-reinsert its ingredients (mirroring recipe.Repo.Update).
func approveRecipe(tx *gorm.DB, rev *revision.Repo, id, actorID uint) error {
	// See Task P2.4 recipePayload; implement create/edit/delete branches here,
	// re-pointing ingredients from the overlay. Entity type string is "recipe".
	return approveGeneric(tx, rev, "recipe", "recipes", id, actorID)
}

func approveCase(tx *gorm.DB, rev *revision.Repo, id, actorID uint) error {
	return approveGeneric(tx, rev, "case", "cases", id, actorID)
}
```

> Implementation note for Step 3: `approveGeneric` is a small helper that reads `review_state`, `pending_json`, `pending_delete` for the row, then branches exactly like `approveDoctor` — delete → hard delete + revision `delete`; edit → unmarshal overlay into the entity, `Save`, revision `update`; pending → flip to `approved`, revision `create`; else `errNotPending`. For recipes, unmarshal into `recipePayload` and additionally delete+reinsert ingredients. Keep `scanItems`/`scanChild` as thin `Rows()`-scanning helpers that map into `Item` and set `Action` from the row state (`pending_delete → delete`, `pending_json != nil → update`, else `create`). Write these out fully; do not leave them stubbed.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/review/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/review/review.go internal/review/review_test.go && rtk git commit -m "feat(p2): add review queue with approve/reject/bulk"
```

---

### Task P2.8: `review` HTTP adapter + router wiring

**Files:**
- Create: `internal/review/handler.go`
- Modify: `internal/router/router.go`
- Test: `internal/review/handler_test.go`

**Interfaces:**
- Consumes: `review.Repo`, `auth.RequireRole`, `httpx`.
- Produces: admin-only routes `GET /api/review/queue`, `POST /api/review/:entityType/:entityId/approve`, `POST /api/review/:entityType/:entityId/reject`, `POST /api/review/doctors/:id/approve-tree`.

- [ ] **Step 1: Write the failing test**

```go
func TestQueueRequiresCentralAdmin(t *testing.T) {
	env := newReviewAPI(t) // wires review.RegisterRoutes with admin+editor sessions
	if rec := env.doAsEditor(t, "GET", "/api/review/queue", ""); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	if rec := env.doAsAdmin(t, "GET", "/api/review/queue", ""); rec.Code != 200 {
		t.Fatalf("admin must be allowed, got %d", rec.Code)
	}
}

func TestApproveEndpointPromotes(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: 1, ReviewState: model.ReviewPending}
	env.g.Create(&d)
	rec := env.doAsAdmin(t, "POST", "/api/review/doctor/"+itoa(d.ID)+"/approve", "")
	if rec.Code != 200 {
		t.Fatalf("approve status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got model.Doctor
	env.g.First(&got, d.ID)
	if got.ReviewState != model.ReviewApproved {
		t.Fatalf("state = %q", got.ReviewState)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/review/ -run TestQueueRequiresCentralAdmin -v`
Expected: FAIL — `RegisterRoutes` undefined.

- [ ] **Step 3: Write the handler and wire the router**

```go
// internal/review/handler.go
package review

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

func RegisterRoutes(r gin.IRouter, repo *Repo) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/review/queue", admin, queueHandler(repo))
	r.POST("/api/review/:entityType/:entityId/approve", admin, approveHandler(repo))
	r.POST("/api/review/:entityType/:entityId/reject", admin, rejectHandler(repo))
	r.POST("/api/review/doctors/:id/approve-tree", admin, approveTreeHandler(repo))
}

func queueHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := repo.Queue(nil)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "queue_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, items)
	}
}

func approveHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		if err := repo.Approve(c.Param("entityType"), uint(id), user.ID); err != nil {
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "approved"})
	}
}

func rejectHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Reason == "" {
			httpx.Err(c, http.StatusBadRequest, "reason_required", "a rejection reason is required")
			return
		}
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		if err := repo.Reject(c.Param("entityType"), uint(id), user.ID, body.Reason); err != nil {
			httpx.Err(c, http.StatusBadRequest, "reject_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "rejected"})
	}
}

func approveTreeHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid doctor id")
			return
		}
		n, err := repo.ApproveDoctorTree(uint(id), user.ID)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"approved": n})
	}
}
```

In `internal/router/router.go`, build the shared revision + review repos and thread them everywhere:

```go
rev := revision.NewRepo(deps.DB, deps.Clk)
// ...
doctor.RegisterRoutes(api, doctor.NewRepo(deps.DB, deps.Clk, rev), deps.Media)
recipe.RegisterRoutes(api, recipe.NewRepo(deps.DB, deps.Clk, rev))
caserec.RegisterRoutes(api, caserec.NewRepo(deps.DB, deps.Clk, rev), deps.Media)
review.RegisterRoutes(api, review.NewRepo(deps.DB, rev))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/review/ -v && rtk go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
rtk git add internal/review/handler.go internal/review/handler_test.go internal/router/router.go && rtk git commit -m "feat(p2): expose review queue endpoints and wire router"
```

---

### Task P2.9: Full P2 verification, ADR, CONTEXT.md

**Files:**
- Modify: `CONTEXT.md`
- Create: `docs/adr/ADR-0002-on-row-pending-model-b.md`

- [ ] **Step 1: Run the whole suite**

Run: `rtk go test ./...`
Expected: PASS. If any pre-existing test broke on the `NewRepo` arity change, fix the call site (not the assertion).

- [ ] **Step 2: Write ADR-0002**

Create `docs/adr/ADR-0002-on-row-pending-model-b.md` capturing: decision = Model-B (every editor save queued) + on-row pending columns (chosen over a clean live-queue table, which cannot give a pending new parent a real id for its children); consequences = public filter must add `review_state = approved`; alternatives considered. Reference the scope doc.

- [ ] **Step 3: Update CONTEXT.md**

Add a dated bullet under `## Status` describing P2 (approval-before-publish, on-row pending Model-B, Revision history, `review_state` public filter). Under `## Data model (summary)`, note the four pending columns on Doctor/Recipe/Case and the new `Revision` table; bump the entity count line if you count `Revision`.

- [ ] **Step 4: Commit**

```bash
rtk git add CONTEXT.md docs/adr/ADR-0002-on-row-pending-model-b.md && rtk git commit -m "docs(p2): ADR-0002 + CONTEXT for approval/history"
```

- [ ] **Step 5: Merge to main** (after review, per finishing-a-development-branch)

```bash
rtk git checkout main && rtk git merge --no-ff feat/p2-approval-history && rtk git push
```

---

# Phase P3 — Year locking

**Branch:** `feat/p3-year-locking` (create from updated `main` after P2 merges).

**Goal.** Freeze a whole `data_year` so its Recipe/Case rows become read-only, except the admin PDPA-erasure path. A year may be locked only when its pending queue is empty.

### Contracts

```go
// internal/model/model.go
type YearLock struct {
    DataYear int       `gorm:"primaryKey"`
    LockedAt time.Time `gorm:"not null"`
    LockedBy uint      `gorm:"not null"`
}

// internal/yearlock/yearlock.go
func NewRepo(g *gorm.DB, clk clock.Clock) *Repo
func (r *Repo) IsLocked(dataYear int) (bool, error)
func (r *Repo) Lock(dataYear int, actorID uint) error   // errs if pending rows exist for the year
func (r *Repo) Unlock(dataYear int) error
func (r *Repo) List() ([]model.YearLock, error)

var ErrYearLocked   = errors.New("yearlock: data_year is locked")
var ErrPendingInYear = errors.New("yearlock: cannot lock a year with pending changes")
```

---

### Task P3.1: `YearLock` model + migration

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/model_test.go` (extend)

- [ ] **Step 1: Write the failing test**

```go
func TestYearLockMigrates(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1}).Error; err != nil {
		t.Fatalf("insert year lock: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/model/ -run TestYearLockMigrates -v`
Expected: FAIL — `model.YearLock` undefined.

- [ ] **Step 3: Add the struct + migration**

Add the `YearLock` struct from Contracts and append `&YearLock{}` to `AutoMigrate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/model/ -run TestYearLockMigrates -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/model/ && rtk git commit -m "feat(p3): add YearLock model"
```

---

### Task P3.2: `yearlock` package — lock/unlock/list with pending-empty guard

**Files:**
- Create: `internal/yearlock/yearlock.go`
- Test: `internal/yearlock/yearlock_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/yearlock/yearlock_test.go
package yearlock_test

import (
	"errors"
	"path/filepath"
	"testing"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/yearlock"
)

func TestLockRefusedWhenPendingExists(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	model.AutoMigrate(g)
	// A pending recipe in year 2567.
	g.Create(&model.Recipe{Code: "R1", Name: "x", DoctorID: 1, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2567, ReviewState: model.ReviewPending})
	repo := yearlock.NewRepo(g, clock.Real{})

	if err := repo.Lock(2567, 1); !errors.Is(err, yearlock.ErrPendingInYear) {
		t.Fatalf("want ErrPendingInYear, got %v", err)
	}
}

func TestLockThenIsLocked(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	model.AutoMigrate(g)
	repo := yearlock.NewRepo(g, clock.Real{})
	if err := repo.Lock(2567, 1); err != nil {
		t.Fatalf("lock: %v", err)
	}
	locked, _ := repo.IsLocked(2567)
	if !locked {
		t.Fatalf("2567 should be locked")
	}
	open, _ := repo.IsLocked(2568)
	if open {
		t.Fatalf("2568 should be open")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/yearlock/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// internal/yearlock/yearlock.go
package yearlock

import (
	"errors"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

var (
	ErrYearLocked    = errors.New("yearlock: data_year is locked")
	ErrPendingInYear = errors.New("yearlock: cannot lock a year with pending changes")
)

type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

func (r *Repo) IsLocked(dataYear int) (bool, error) {
	var n int64
	err := r.g.Model(&model.YearLock{}).Where("data_year = ?", dataYear).Count(&n).Error
	return n > 0, err
}

// Lock freezes a year. It refuses if any Recipe/Case in the year is still pending,
// so a locked year always means "final approved state".
func (r *Repo) Lock(dataYear int, actorID uint) error {
	pending := func(table string) (bool, error) {
		var n int64
		err := r.g.Table(table).
			Where("data_year = ? AND (review_state = ? OR (pending_json IS NOT NULL AND rejection_reason IS NULL) OR (pending_delete = ? AND rejection_reason IS NULL))",
				dataYear, model.ReviewPending, true).
			Count(&n).Error
		return n > 0, err
	}
	for _, table := range []string{"recipes", "cases"} {
		has, err := pending(table)
		if err != nil {
			return err
		}
		if has {
			return ErrPendingInYear
		}
	}
	return r.g.Create(&model.YearLock{DataYear: dataYear, LockedAt: r.clk.Now(), LockedBy: actorID}).Error
}

func (r *Repo) Unlock(dataYear int) error {
	return r.g.Where("data_year = ?", dataYear).Delete(&model.YearLock{}).Error
}

func (r *Repo) List() ([]model.YearLock, error) {
	var out []model.YearLock
	err := r.g.Order("data_year DESC").Find(&out).Error
	return out, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/yearlock/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/yearlock/yearlock.go internal/yearlock/yearlock_test.go && rtk git commit -m "feat(p3): add yearlock repo with pending-empty guard"
```

---

### Task P3.3: Write guard in recipe + case, admin-erasure exempt

**Files:**
- Modify: `internal/recipe/recipe.go`, `internal/caserec/case.go`
- Modify: `internal/router/router.go` (inject `*yearlock.Repo`)
- Test: `internal/recipe/recipe_test.go` (extend)

**Interfaces:**
- Consumes: `yearlock.Repo`.
- Produces: recipe/case repos gain a `lock *yearlock.Repo` field; write methods return `yearlock.ErrYearLocked` when the row's `data_year` is locked, except when `immediate` (admin) is on the PDPA-erasure path.

> **Rider (scope P3 decision 2):** PDPA erasure/unpublish overrides any lock. The admin delete (`immediate = true`) is that path, so the guard applies to **editor** writes and to non-erasure admin edits, but the admin **delete** is allowed through even on a locked year. Model the guard as: block when `!immediate && locked`; additionally block an admin *edit* (not delete) into a locked year, but always allow admin *delete*.

- [ ] **Step 1: Write the failing test**

```go
func TestEditorWriteRefusedInLockedYear(t *testing.T) {
	env := newRecipeAPI(t)
	env.g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1})
	body := `{"code":"R2","name":"x","doctorCode":"D1","indication":"-","preparation":"-","usage":"-","dataYear":2567,"ingredients":[{"herbId":1,"amount":"1","unit":"g"}]}`
	rec := env.doAsEditor(t, "POST", "/api/recipes", body)
	if rec.Code != 409 {
		t.Fatalf("editor create in locked year must be 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminEraseAllowedInLockedYear(t *testing.T) {
	env := newRecipeAPI(t) // seeds recipe id 1 in year 2567
	env.g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1})
	rec := env.doAsAdmin(t, "DELETE", "/api/recipes/1", "")
	if rec.Code != 200 && rec.Code != 204 {
		t.Fatalf("admin erasure must bypass the lock, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/recipe/ -run 'TestEditorWriteRefusedInLockedYear|TestAdminEraseAllowedInLockedYear' -v`
Expected: FAIL — no guard; both writes go through.

- [ ] **Step 3: Add the guard**

Add a `lock *yearlock.Repo` field to `recipe.Repo` and `caserec.Repo`, set via `NewRepo(g, clk, rev, lock)`. At the top of `Create`/`Update` (both recipe and case), guard editor writes and admin non-delete edits:

```go
func (r *Repo) guardYear(dataYear int, immediate bool) error {
	if immediate {
		return nil // admin edits are the approver's call; delete is erasure (always allowed)
	}
	locked, err := r.lock.IsLocked(dataYear)
	if err != nil {
		return err
	}
	if locked {
		return yearlock.ErrYearLocked
	}
	return nil
}
```

Call `if err := r.guardYear(rec.DataYear, immediate); err != nil { return err }` at the start of `Create` and `Update`. Leave `Delete` unguarded (admin erasure and editor pending-delete both permitted; an editor pending-delete on a locked year is a queued proposal, not a mutation of frozen data — acceptable, and it cannot be approved because approval would mutate a locked row; if stricter behavior is wanted, also guard editor delete). In the handlers, map `yearlock.ErrYearLocked` to HTTP 409:

```go
if errors.Is(err, yearlock.ErrYearLocked) {
	httpx.Err(c, http.StatusConflict, "year_locked", "this data year is locked")
	return
}
```

Wire `yearlock.NewRepo(deps.DB, deps.Clk)` in `router.go` and pass into recipe/caserec `NewRepo`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/recipe/ ./internal/caserec/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/recipe/ internal/caserec/ internal/router/router.go && rtk git commit -m "feat(p3): refuse editor writes into locked years, exempt admin erasure"
```

---

### Task P3.4: yearlock HTTP adapter + router + CONTEXT

**Files:**
- Create: `internal/yearlock/handler.go`
- Modify: `internal/router/router.go`, `CONTEXT.md`
- Test: `internal/yearlock/handler_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLockEndpointAdminOnly(t *testing.T) {
	env := newYearLockAPI(t)
	if rec := env.doAsEditor(t, "POST", "/api/year-locks", `{"dataYear":2567}`); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	if rec := env.doAsAdmin(t, "POST", "/api/year-locks", `{"dataYear":2567}`); rec.Code != 201 {
		t.Fatalf("admin lock status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/yearlock/ -run TestLockEndpointAdminOnly -v`
Expected: FAIL — `RegisterRoutes` undefined.

- [ ] **Step 3: Write the handler + wire router**

```go
// internal/yearlock/handler.go
package yearlock

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

func RegisterRoutes(r gin.IRouter, repo *Repo) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/year-locks", admin, listHandler(repo))
	r.POST("/api/year-locks", admin, lockHandler(repo))
	r.DELETE("/api/year-locks/:dataYear", admin, unlockHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		locks, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "list_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, locks)
	}
}

func lockHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			DataYear int `json:"dataYear"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.DataYear == 0 {
			httpx.Err(c, http.StatusBadRequest, "bad_year", "dataYear is required")
			return
		}
		if err := repo.Lock(body.DataYear, user.ID); err != nil {
			if errors.Is(err, ErrPendingInYear) {
				httpx.Err(c, http.StatusConflict, "pending_in_year", "clear the pending queue for this year first")
				return
			}
			httpx.Err(c, http.StatusInternalServerError, "lock_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusCreated, gin.H{"dataYear": body.DataYear})
	}
}

func unlockHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		year, err := strconv.Atoi(c.Param("dataYear"))
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_year", "invalid data year")
			return
		}
		if err := repo.Unlock(year); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "unlock_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"dataYear": year})
	}
}
```

Register in `router.go`: `yearlock.RegisterRoutes(api, lockRepo)` where `lockRepo` is the same instance injected into recipe/caserec.

- [ ] **Step 4: Run tests + update CONTEXT.md**

Run: `rtk go test ./... && rtk go build ./...`
Then add a dated `## Status` bullet and note the `YearLock` table in `## Data model (summary)`.

- [ ] **Step 5: Commit + merge**

```bash
rtk git add internal/yearlock/handler.go internal/yearlock/handler_test.go internal/router/router.go CONTEXT.md && rtk git commit -m "feat(p3): year-lock endpoints + CONTEXT"
rtk git checkout main && rtk git merge --no-ff feat/p3-year-locking && rtk git push
```

---

# Phase P4 — Bulk import (canonical template)

**Branch:** `feat/p4-bulk-import`.

**Goal.** Load client data in bulk from one canonical Excel template (the spreadsheet twin of form Sheets A/B/C), through the same domain services and rules as manual entry. Imports are admin actions → rows enter `approved` immediately but stay consent-gated. Insert-only: existing `code`s are skipped and reported. Refuse imports into a P3-locked year. Per-batch undo.

### Contracts

```go
// internal/model/model.go
type ImportBatch struct {
    ID         uint      `gorm:"primaryKey"`
    ImportedBy uint      `gorm:"not null"`
    ImportedAt time.Time `gorm:"not null"`
    SourceFile string    `gorm:"not null"`
    RowCount   int
    Status     string    `gorm:"not null"` // committed | undone
}
// Doctor/Recipe/Case gain: BatchID *uint `gorm:"index"`

// internal/importer/importer.go
type Report struct {
    DryRun       bool          `json:"dryRun"`
    DoctorsNew   int           `json:"doctorsNew"`
    RecipesNew   int           `json:"recipesNew"`
    CasesNew     int           `json:"casesNew"`
    Skipped      []SkippedRow  `json:"skipped"`  // existing codes
    Errors       []RowError    `json:"errors"`   // validation failures
    BatchID      *uint         `json:"batchId,omitempty"` // set on a committed run
}
func NewImporter(g *gorm.DB, clk clock.Clock, doc *doctor.Repo, rec *recipe.Repo, cas *caserec.Repo, herbs *herb.Repo, lock *yearlock.Repo) *Importer
func (im *Importer) Run(file io.Reader, sourceName string, actorID uint, dryRun bool) (*Report, error)
func (im *Importer) Undo(batchID uint) error
```

---

### Task P4.1: ImportBatch model + batch tags

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/model_test.go` (extend)

- [ ] **Step 1: Write the failing test**

```go
func TestImportBatchMigrates(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "ib.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := model.ImportBatch{ImportedBy: 1, SourceFile: "f.xlsx", RowCount: 3, Status: "committed"}
	if err := g.Create(&b).Error; err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, BatchID: &b.ID}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("insert tagged doctor: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/model/ -run TestImportBatchMigrates -v`
Expected: FAIL — `model.ImportBatch` / `BatchID` undefined.

- [ ] **Step 3: Add struct, `BatchID` fields, migration**

Add the `ImportBatch` struct, add `BatchID *uint \`gorm:"index"\`` to `Doctor`, `Recipe`, `Case`, and append `&ImportBatch{}` to `AutoMigrate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/model/ -run TestImportBatchMigrates -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/model/ && rtk git commit -m "feat(p4): add ImportBatch model + batch tags"
```

---

### Task P4.2: Template parser

**Files:**
- Create: `internal/importer/template.go`, `internal/importer/parse.go`
- Test: `internal/importer/parse_test.go`

**Interfaces:**
- Produces: `parseWorkbook(r io.Reader) (*parsed, error)` where `parsed` holds typed `doctorRow`/`recipeRow`/`caseRow` slices keyed by the sheet. Column order is fixed by `template.go` (Sheets A/B/C twin, data spec §5). Reuses `excelize` (already a dep; export.go proves the write side — this is the read side).

- [ ] **Step 1: Write the failing test**

Build a fixture workbook in-test with `excelize.NewFile()` (three sheets `Doctors`, `Recipes`, `Cases`), write header + one data row each, `f.WriteToBuffer()`, then parse:

```go
func TestParseReadsThreeSheets(t *testing.T) {
	buf := buildFixtureWorkbook(t) // helper writes one doctor, one recipe, one case
	p, err := importer.ParseWorkbook(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.Doctors) != 1 || p.Doctors[0].Code != "D1" {
		t.Fatalf("doctors = %+v", p.Doctors)
	}
	if len(p.Recipes) != 1 || p.Recipes[0].DoctorCode != "D1" {
		t.Fatalf("recipes = %+v", p.Recipes)
	}
	if len(p.Cases) != 1 || p.Cases[0].RecipeCode != "R1" {
		t.Fatalf("cases = %+v", p.Cases)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/importer/ -run TestParseReadsThreeSheets -v`
Expected: FAIL — package/`ParseWorkbook` undefined.

- [ ] **Step 3: Write template + parser**

`template.go` declares the exact column headers per sheet (matching the standard form; use full English field names). `parse.go`:

```go
// internal/importer/parse.go
package importer

import (
	"io"

	"github.com/xuri/excelize/v2"
)

type DoctorRow struct {
	Code, FullName, Specialty, Status string
	DistrictID, FirstYear             int
	// ... remaining Doctor form fields, in template column order
}
type RecipeRow struct {
	Code, Name, DoctorCode, Indication, Preparation, Usage string
	DataYear                                               int
	Ingredients                                            []IngredientRow
}
type IngredientRow struct {
	HerbName, Amount, Unit string
}
type CaseRow struct {
	RecipeCode, Condition, Result string
	DataYear                      int
	// ... remaining Case fields
}
type Parsed struct {
	Doctors []DoctorRow
	Recipes []RecipeRow
	Cases   []CaseRow
}

func ParseWorkbook(r io.Reader) (*Parsed, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := &Parsed{}
	if err := readSheet(f, "Doctors", func(row []string) { out.Doctors = append(out.Doctors, doctorFromRow(row)) }); err != nil {
		return nil, err
	}
	if err := readSheet(f, "Recipes", func(row []string) { out.Recipes = append(out.Recipes, recipeFromRow(row)) }); err != nil {
		return nil, err
	}
	if err := readSheet(f, "Cases", func(row []string) { out.Cases = append(out.Cases, caseFromRow(row)) }); err != nil {
		return nil, err
	}
	return out, nil
}

// readSheet iterates data rows (skipping the header) and calls fn per non-empty row.
func readSheet(f *excelize.File, sheet string, fn func(row []string)) error {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return err
	}
	for i, row := range rows {
		if i == 0 || len(row) == 0 {
			continue // header / blank
		}
		fn(row)
	}
	return nil
}
```

Write `doctorFromRow`/`recipeFromRow`/`caseFromRow` as explicit column-index mappers (`atoiSafe(row[4])` etc.) matching `template.go`. Ingredients on a recipe row: either repeated ingredient columns or a linked `Ingredients` sub-sheet — pick repeated columns for the v1 template and document it in `template.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/importer/ -run TestParseReadsThreeSheets -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/importer/template.go internal/importer/parse.go internal/importer/parse_test.go && rtk git commit -m "feat(p4): canonical template parser"
```

---

### Task P4.3: Dry-run validation report

**Files:**
- Create: `internal/importer/importer.go`
- Test: `internal/importer/importer_test.go`

**Interfaces:**
- Consumes: `Parsed`, the domain repos, `yearlock.Repo`.
- Produces: `Report`, `NewImporter`, `(*Importer).Run(..., dryRun=true)` (no writes; validates codes, links, locked years, duplicates).

- [ ] **Step 1: Write the failing test**

```go
func TestDryRunReportsDuplicatesAndLockedYears(t *testing.T) {
	env := newImporterEnv(t) // wires all domain repos on a temp DB
	env.g.Create(&model.Doctor{Code: "D1", Photo: "-", FullName: "existing", Specialty: "y", Status: "active", FirstYear: 2568, ReviewState: model.ReviewApproved})
	env.g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1})
	buf := buildFixtureWorkbook(t) // contains doctor D1 (dup) + a recipe in year 2567 (locked)

	rep, err := env.im.Run(buf, "f.xlsx", 1, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(rep.Skipped) == 0 {
		t.Fatalf("expected D1 reported as skipped duplicate")
	}
	if len(rep.Errors) == 0 {
		t.Fatalf("expected a locked-year error for the 2567 recipe")
	}
	// Dry run must not write.
	var n int64
	env.g.Model(&model.ImportBatch{}).Count(&n)
	if n != 0 {
		t.Fatalf("dry run must not create a batch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/importer/ -run TestDryRunReportsDuplicatesAndLockedYears -v`
Expected: FAIL — `Run` undefined.

- [ ] **Step 3: Write validation**

Implement `Run` for the `dryRun` path: for each doctor row check `code` exists → `Skipped`; validate required fields → `Errors`. For each recipe/case row: resolve `DoctorCode`/`RecipeCode` link (must exist in DB or earlier in the same file) → `Errors` if unresolved; check `IsLocked(DataYear)` → `Errors` if locked; check duplicate `code`. Build counts of would-be-new rows. Return the `Report` with no DB writes.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/importer/ -run TestDryRunReportsDuplicatesAndLockedYears -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/importer/importer.go internal/importer/importer_test.go && rtk git commit -m "feat(p4): dry-run validation report"
```

---

### Task P4.4: Commit path (one transaction) + per-batch undo

**Files:**
- Modify: `internal/importer/importer.go`
- Test: `internal/importer/importer_test.go` (extend)

**Interfaces:**
- Produces: `Run(..., dryRun=false)` writes through the domain repos with `immediate=true` (admin), tags rows with the new `BatchID`, all inside one `db.Tx`; `Undo(batchID)` deletes the batch's rows.

- [ ] **Step 1: Write the failing test**

```go
func TestCommitWritesApprovedThenUndo(t *testing.T) {
	env := newImporterEnv(t)
	buf := buildFixtureWorkbook(t) // doctor D1 + recipe R1 + case, all new, unlocked year

	rep, err := env.im.Run(buf, "f.xlsx", 1, false)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if rep.BatchID == nil {
		t.Fatalf("committed run must return a batch id")
	}
	var d model.Doctor
	env.g.Where("code = ?", "D1").First(&d)
	if d.ReviewState != model.ReviewApproved {
		t.Fatalf("imported doctor must be approved, got %q", d.ReviewState)
	}
	if d.ConsentObtained {
		t.Fatalf("imported doctor must default to consent=false (hidden until consent recorded)")
	}

	if err := env.im.Undo(*rep.BatchID); err != nil {
		t.Fatalf("undo: %v", err)
	}
	var n int64
	env.g.Model(&model.Doctor{}).Where("code = ?", "D1").Count(&n)
	if n != 0 {
		t.Fatalf("undo must remove imported doctor")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/importer/ -run TestCommitWritesApprovedThenUndo -v`
Expected: FAIL — commit/undo not implemented.

- [ ] **Step 3: Write commit + undo**

Commit path inside `db.Tx`: create the `ImportBatch` row first; for each new doctor call `doc.Create(&d, actorID, true)` (approved, consent defaults false), set `BatchID`; for each recipe resolve the doctor id, call `rec.Create(...)`, route unknown herbs through the existing pending-herb path (FR-HERB-2) via `herbs`; for each case call `cas.Create(...)`. Skip existing codes (already in the report). Tag every inserted row's `BatchID`. On any hard error, return it so `db.Tx` rolls the whole batch back. `Undo`: in a transaction, delete `Case`/`Recipe`/`Doctor` where `batch_id = ?` (children first for FK safety), then mark the batch `status = "undone"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/importer/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/importer/ && rtk git commit -m "feat(p4): transactional commit + per-batch undo"
```

---

### Task P4.5: importer HTTP adapter + router + CONTEXT

**Files:**
- Create: `internal/importer/handler.go`
- Modify: `internal/router/router.go`, `CONTEXT.md`
- Test: `internal/importer/handler_test.go`

**Interfaces:**
- Produces: admin-only `POST /api/imports?dryRun=true|false` (multipart file upload) and `POST /api/imports/:batchId/undo`.

- [ ] **Step 1: Write the failing test**

```go
func TestImportEndpointAdminOnlyMultipart(t *testing.T) {
	env := newImporterAPI(t)
	body, contentType := multipartWorkbook(t) // builds a fixture .xlsx multipart body
	if rec := env.doAsEditorMultipart(t, "POST", "/api/imports?dryRun=true", body, contentType); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	body2, ct2 := multipartWorkbook(t)
	rec := env.doAsAdminMultipart(t, "POST", "/api/imports?dryRun=true", body2, ct2)
	if rec.Code != 200 {
		t.Fatalf("admin dry run status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/importer/ -run TestImportEndpointAdminOnlyMultipart -v`
Expected: FAIL — `RegisterRoutes` undefined.

- [ ] **Step 3: Write handler + wire router**

Handler: `admin := auth.RequireRole(model.RoleCentralAdmin)`; read `dryRun := c.Query("dryRun") == "true"`; open the multipart file (`c.FormFile("file")`), call `im.Run(f, header.Filename, user.ID, dryRun)`, return the `Report` as JSON. Undo handler parses `:batchId` and calls `im.Undo`. In `router.go`, build the `Importer` with the already-constructed doctor/recipe/caserec/herb/yearlock repos and register.

- [ ] **Step 4: Run tests + update CONTEXT.md**

Run: `rtk go test ./... && rtk go build ./...`
Add a `## Status` bullet and note the `ImportBatch` table + `BatchID` tags in `## Data model (summary)`.

- [ ] **Step 5: Commit + merge**

```bash
rtk git add internal/importer/handler.go internal/importer/handler_test.go internal/router/router.go CONTEXT.md && rtk git commit -m "feat(p4): import endpoints + CONTEXT"
rtk git checkout main && rtk git merge --no-ff feat/p4-bulk-import && rtk git push
```

---

# Phase P5 — District-managed herb catalog

**Branch:** `feat/p5-district-herbs`.

**Goal.** Widen write access to the single shared province-wide herb catalog: a district may **add** herbs and **edit ones it created**; the central admin edits across districts and runs a **merge/alias** tool. A save-time near-duplicate warning nudges against dupes. The P1 pending-herb path stays as a fallback.

### Contracts

```go
// internal/model/model.go — Herb gains:
    CreatedByDistrictID *uint `gorm:"index"`
    AliasOfID           *uint `gorm:"index"` // set => this herb is an alias of AliasOfID

// internal/herb/herb.go
func (r *Repo) Create(h *model.Herb, actor auth.CurrentUser) error       // editor may create
func (r *Repo) Update(h *model.Herb, actor auth.CurrentUser) error       // editor edits own only
func (r *Repo) Merge(aliasID, canonicalID, actorID uint) (int64, error)  // admin only; re-points ingredients
func (r *Repo) NearDuplicates(thaiName string) ([]model.Herb, error)     // save-time warning
var ErrNotOwner = errors.New("herb: district may edit only herbs it created")
```

---

### Task P5.1: Herb provenance + alias columns

**Files:**
- Modify: `internal/model/model.go`
- Test: `internal/model/model_test.go` (extend)

- [ ] **Step 1: Write the failing test**

```go
func TestHerbProvenanceAndAliasColumns(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "h.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	did := uint(3)
	canonical := model.Herb{ThaiName: "ขิง"}
	g.Create(&canonical)
	alias := model.Herb{ThaiName: "ขิงแก่", CreatedByDistrictID: &did, AliasOfID: &canonical.ID}
	if err := g.Create(&alias).Error; err != nil {
		t.Fatalf("insert alias herb: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/model/ -run TestHerbProvenanceAndAliasColumns -v`
Expected: FAIL — fields undefined.

- [ ] **Step 3: Add the two fields**

Add `CreatedByDistrictID *uint` and `AliasOfID *uint` to `Herb`. `AutoMigrate` list is unchanged (same struct); AutoMigrate adds the columns.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/model/ -run TestHerbProvenanceAndAliasColumns -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/model/ && rtk git commit -m "feat(p5): herb provenance + alias columns"
```

---

### Task P5.2: Widen herb write port — create + edit-own + near-dup + merge

**Files:**
- Modify: `internal/herb/herb.go`
- Test: `internal/herb/herb_test.go` (extend/create)

**Interfaces:**
- Consumes: `auth.CurrentUser`, `model`.
- Produces: the herb methods in Contracts.

- [ ] **Step 1: Write the failing test**

```go
func TestEditorCreatesAndEditsOwnHerbOnly(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "h.db"))
	model.AutoMigrate(g)
	repo := herb.NewRepo(g)
	d1, d2 := uint(1), uint(2)
	editor1 := auth.CurrentUser{ID: 10, Role: model.RoleDistrictEditor, DistrictID: &d1}
	editor2 := auth.CurrentUser{ID: 20, Role: model.RoleDistrictEditor, DistrictID: &d2}

	h := model.Herb{ThaiName: "ฟ้าทะลายโจร"}
	if err := repo.Create(&h, editor1); err != nil {
		t.Fatalf("editor create: %v", err)
	}
	if h.CreatedByDistrictID == nil || *h.CreatedByDistrictID != d1 {
		t.Fatalf("create must stamp provenance")
	}
	// Another district may not edit it.
	h.ThaiName = "แก้ไข"
	if err := repo.Update(&h, editor2); !errors.Is(err, herb.ErrNotOwner) {
		t.Fatalf("want ErrNotOwner, got %v", err)
	}
	// Owner may.
	if err := repo.Update(&h, editor1); err != nil {
		t.Fatalf("owner update: %v", err)
	}
}

func TestMergeRepointsIngredients(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "h.db"))
	model.AutoMigrate(g)
	repo := herb.NewRepo(g)
	canonical := model.Herb{ThaiName: "ขิง"}
	dup := model.Herb{ThaiName: "ขิง (ซ้ำ)"}
	g.Create(&canonical)
	g.Create(&dup)
	g.Create(&model.Recipe{Code: "R1", Name: "x", DoctorID: 1, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568})
	g.Create(&model.Ingredient{RecipeID: 1, HerbID: &dup.ID, Amount: "1", Unit: "g"})

	n, err := repo.Merge(dup.ID, canonical.ID, 99)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if n != 1 {
		t.Fatalf("re-pointed = %d, want 1", n)
	}
	var ing model.Ingredient
	g.First(&ing, 1)
	if ing.HerbID == nil || *ing.HerbID != canonical.ID {
		t.Fatalf("ingredient not re-pointed to canonical")
	}
	var alias model.Herb
	g.First(&alias, dup.ID)
	if alias.AliasOfID == nil || *alias.AliasOfID != canonical.ID {
		t.Fatalf("dup must be marked alias of canonical")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/herb/ -v`
Expected: FAIL — new signatures / `ErrNotOwner` / `Merge` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/herb/herb.go (additions/changes)
var ErrNotOwner = errors.New("herb: district may edit only herbs it created")

func (r *Repo) Create(h *model.Herb, actor auth.CurrentUser) error {
	if actor.Role == model.RoleDistrictEditor {
		h.CreatedByDistrictID = actor.DistrictID
	}
	return r.g.Create(h).Error
}

func (r *Repo) Update(h *model.Herb, actor auth.CurrentUser) error {
	var existing model.Herb
	if err := r.g.First(&existing, h.ID).Error; err != nil {
		return err
	}
	if actor.Role == model.RoleDistrictEditor {
		if existing.CreatedByDistrictID == nil || actor.DistrictID == nil || *existing.CreatedByDistrictID != *actor.DistrictID {
			return ErrNotOwner
		}
	}
	h.CreatedByDistrictID = existing.CreatedByDistrictID // provenance is immutable
	h.AliasOfID = existing.AliasOfID
	return r.g.Save(h).Error
}

// Merge marks alias an alias of canonical and re-points every ingredient. Admin only
// (enforced at the handler via RequireRole). Returns rows re-pointed.
func (r *Repo) Merge(aliasID, canonicalID, actorID uint) (int64, error) {
	var n int64
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		res := tx.Model(&model.Ingredient{}).Where("herb_id = ?", aliasID).Update("herb_id", canonicalID)
		if res.Error != nil {
			return res.Error
		}
		n = res.RowsAffected
		return tx.Model(&model.Herb{}).Where("id = ?", aliasID).Update("alias_of_id", canonicalID).Error
	})
	return n, err
}

// NearDuplicates returns catalogued herbs whose Thai name is a prefix/substring match,
// for a save-time warning. Portable LIKE (no SQLite-only feature).
func (r *Repo) NearDuplicates(thaiName string) ([]model.Herb, error) {
	var out []model.Herb
	err := r.g.Where("alias_of_id IS NULL AND thai_name LIKE ?", "%"+thaiName+"%").
		Limit(5).Find(&out).Error
	return out, err
}
```

Add `errors`, `auth`, `db` imports as needed. `NewRepo` stays `NewRepo(g *gorm.DB)` (herb has no clock/audit today).

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/herb/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
rtk git add internal/herb/herb.go internal/herb/herb_test.go && rtk git commit -m "feat(p5): widen herb writes, add merge/alias + near-dup check"
```

---

### Task P5.3: Herb handler gating + merge endpoint + router + CONTEXT

**Files:**
- Modify: `internal/herb/handler.go`, `internal/router/router.go`, `CONTEXT.md`
- Test: `internal/herb/handler_test.go` (extend)

**Interfaces:**
- Produces: `POST /api/herbs` and `PUT /api/herbs/:id` allow authenticated editors (ownership enforced in repo); `POST /api/herbs/:aliasId/merge/:canonicalId` and the reconcile route stay `central_admin` only; each write returns a `nearDuplicates` warning list.

- [ ] **Step 1: Write the failing test**

```go
func TestEditorMayCreateHerbButNotMerge(t *testing.T) {
	env := newHerbAPI(t)
	if rec := env.doAsEditor(t, "POST", "/api/herbs", `{"thaiName":"กระชาย"}`); rec.Code != 201 {
		t.Fatalf("editor create herb status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec := env.doAsEditor(t, "POST", "/api/herbs/2/merge/1", ""); rec.Code != 403 {
		t.Fatalf("editor merge must be forbidden, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/herb/ -run TestEditorMayCreateHerbButNotMerge -v`
Expected: FAIL — herb create is currently `RequireRole("central_admin")` (returns 403 for editors).

- [ ] **Step 3: Change the gating**

In `internal/herb/handler.go`, split the routes: create/update use `auth.RequireAuth()` (ownership enforced by `repo.Update` → map `herb.ErrNotOwner` to 403); the merge and reconcile routes keep `auth.RequireRole(model.RoleCentralAdmin)`. Add the merge handler (parse `:aliasId`/`:canonicalId`, call `repo.Merge`). On create/update success, include `nearDuplicates` from `repo.NearDuplicates(h.ThaiName)` in the response body so the UI can warn. Pass `auth.UserFrom(c)` into `repo.Create/Update`.

- [ ] **Step 4: Run tests + update CONTEXT.md**

Run: `rtk go test ./... && rtk go build ./...`
Add a `## Status` bullet and note herb `CreatedByDistrictID`/`AliasOfID` + the merge/alias governance in `## Data model (summary)`. Note FR-HERB-1 ownership rule change.

- [ ] **Step 5: Commit + merge**

```bash
rtk git add internal/herb/handler.go internal/herb/handler_test.go internal/router/router.go CONTEXT.md && rtk git commit -m "feat(p5): district herb write access + merge endpoint + CONTEXT"
rtk git checkout main && rtk git merge --no-ff feat/p5-district-herbs && rtk git push
```

---

## Self-Review

**1. Spec coverage** (against `2026-08-04-p2-p5-scope.md`):

- P2 Model-B gate every change → P2.3–P2.5 (editor writes queue). ✓
- P2 per-record granularity + bulk button → P2.7 `ApproveDoctorTree`. ✓
- P2 consent ∧ review independent → P2.6 keeps consent gate, adds `review_state` alongside. ✓
- P2 on-row pending storage (create=real pending row, edit=overlay, stable ids) → P2.3–P2.5. ✓
- P2 editors queue, admin immediate → `immediate` branch, P2.3–P2.5. ✓
- P2 reject returns to editor (create stays, edit keeps overlay, delete clears flag) → P2.7 `Reject`. ✓
- P2 full-snapshot Revision per approved change + reject logged → P2.2 + `rev.Append` calls. ✓
- P3 lock-only, reconstruct from Revision+backup (no materialized snapshot) → P3.2 (lock row only). ✓
- P3 Recipe/Case only, Doctor exempt → guard only in recipe/caserec (P3.3). ✓
- P3 PDPA erasure overrides lock → admin delete unguarded (P3.3). ✓
- P3 lock only when pending queue empty → P3.2 `ErrPendingInYear`. ✓
- P4 canonical template only → P4.2 fixed columns. ✓
- P4 imported = approved + consent-gated → P4.4 (`immediate=true`, consent defaults false). ✓
- P4 insert-only, skip+report dupes, idempotent → P4.3/P4.4 `Skipped`. ✓
- P4 write through domain services (validation, linking, pending-herb) → P4.4 uses repos. ✓
- P4 refuse into P3-locked year → P4.3 dry-run error + P4.4 commit guard. ✓
- P4 per-batch undo → P4.4 `Undo`. ✓
- P5 one shared catalog, widen write → P5.2 (no per-district catalog). ✓
- P5 add + edit-own + admin-merge + near-dup warning → P5.2. ✓
- P5 merge re-points `Ingredient.HerbID` → P5.2 `Merge`. ✓
- Postgres migration → intentionally out of scope (not planned). ✓
- Every phase updates CONTEXT.md + ADR at funding → P2.9, P3.4, P4.5, P5.3. ✓

**2. Placeholder scan:** The two spots that describe rather than fully inline code — P2.7 `approveGeneric`/`scanItems` and the P4 commit path — carry explicit "write these out fully; do not leave them stubbed" notes plus the exact branch logic and the sibling `approveDoctor` as a complete worked template. Flag for the implementer: treat those notes as required work, not optional.

**3. Type consistency:** `NewRepo` arity changes are consistent across the plan — doctor/recipe/caserec gain `rev` in P2 and recipe/caserec additionally gain `lock` in P3; every call site (`router.go`, test helpers) is updated in the same task that changes the signature. `immediate bool` is the last write-method param throughout. `model.Review*`/`model.Action*`/`model.Role*` constants are defined once in P2.1 and reused everywhere. `Item`, `Report`, `Parsed` field names match between their Contracts block and their tests.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-04-p2-p5-implementation.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration. Matches this repo's "implement with agents" orchestrator workflow (builder → verifier, Build↔Verify capped at 2 rounds).
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints for review.

Which approach — and do you want to start with **P2 only** (fundable unit) or run straight through P2→P5?
