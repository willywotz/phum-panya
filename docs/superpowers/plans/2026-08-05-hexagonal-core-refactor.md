# Hexagonal Core Refactor (Sub-project A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Invert dependencies so the domain core imports no infrastructure and every HTTP handler depends on a port interface, not on `*gorm.DB` or a concrete `*Repo`.

**Architecture:** Keep the feature-package layout. Each feature handler consumes a small **port interface** it declares; the existing GORM `Repo` becomes the driven adapter that implements it (Go structural typing). Shared infrastructure (`media`, `auth` sessions/throttle) is turned into interfaces so sub-projects B and C can swap adapters. `internal/model` becomes a pure domain-entity package. This is a pure refactor — no behavior change, no API change.

**Tech Stack:** Go 1.26, gin, GORM (glebarez SQLite driver), existing table-driven / integration tests over a temp SQLite DB.

## Global Constraints

- TDD is mandatory: write the failing test, confirm it fails, make it pass, refactor. (project rule)
- No behavior change and no HTTP API change. The existing test suite is the safety net and MUST stay green after every task. Run `rtk go test ./...`.
- Never commit to `main`. All work on branch `refactor/hexagonal-core` (already created).
- API route strings are unchanged (full English names — project rule).
- Google Go / Uber Go style. American English names. Organized imports. Run `goimports -w` on touched files.
- Build must pass with `CGO_ENABLED=0 go build ./...`.
- Port interfaces are declared at the consumer and unexported unless another package needs them. The concrete `*Repo` satisfies them implicitly — do not add explicit "implements" wiring beyond a compile-time `var _ port = (*Repo)(nil)` assertion.
- After the last task: update `CONTEXT.md`, commit, then integrate.

---

## File Structure

- `internal/model/model.go` — loses `AutoMigrate` + `BackfillRecipePhotos`; keeps pure entity structs (imports only `time`).
- `internal/db/migrate.go` — **new**; holds `AutoMigrate` + `BackfillRecipePhotos` (moved). `db` imports `model`.
- `internal/media/store.go` — `Store` becomes an interface; the filesystem implementation is renamed `LocalStore` with a `NewLocalStore` constructor.
- `internal/<feature>/handler.go` — each declares an unexported `repository` (and, where used, `mediaStore`) interface; `RegisterRoutes` and per-handler closures take the interface.
- `internal/<feature>/port_test.go` — **new** per feature; internal (`package <feature>`) compile-time assertion that `*Repo` satisfies the port.
- `internal/auth/` — `Sessions`, `Limiter`, `Users` interfaces; `gormUsers` adapter; `LoadUser` and `RegisterRoutes` take interfaces.
- `internal/router/router.go` — `Deps.Media`, `Deps.Store`, `Deps.Throttle` become interface types; `NewEngine` builds the `auth` user adapter from `Deps.DB`.
- `cmd/server/main.go` — constructs `media.NewLocalStore(...)`; calls `db.AutoMigrate` / `db.BackfillRecipePhotos`.

---

### Task 1: Make `internal/model` pure — move migration into `internal/db`

**Files:**
- Create: `internal/db/migrate.go`
- Modify: `internal/model/model.go` (remove `AutoMigrate`, `BackfillRecipePhotos`, drop `gorm` import)
- Modify: `cmd/server/main.go:124,127,197`
- Modify: all call sites of `model.AutoMigrate` / `model.BackfillRecipePhotos` (≈30 test files + main) via find/replace + `goimports`

**Interfaces:**
- Produces: `db.AutoMigrate(g *gorm.DB) error`, `db.BackfillRecipePhotos(g *gorm.DB) error` (same bodies as the current `model` functions).

- [ ] **Step 1: Write the failing test** — add an internal assertion file `internal/model/pure_test.go`:

```go
package model_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestModelHasNoGormImport guards the hexagonal dependency rule: the domain
// entity package must not import any infrastructure.
func TestModelHasNoGormImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "model.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse model.go: %v", err)
	}
	for _, imp := range f.Imports {
		if strings.Contains(imp.Path.Value, "gorm") {
			t.Fatalf("model.go must not import gorm, found %s", imp.Path.Value)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/model/ -run TestModelHasNoGormImport`
Expected: FAIL (`model.go` still imports `gorm.io/gorm`).

- [ ] **Step 3: Move the functions.** Create `internal/db/migrate.go` with the two functions moved verbatim from `model.go` (lines 225–end), changing the package to `db` and adding imports `gorm.io/gorm` and `phum-panya/internal/model`. In `model.go`, delete both functions and the now-unused `gorm` import (keep `time`).

```go
// internal/db/migrate.go
package db

import (
	"gorm.io/gorm"

	"phum-panya/internal/model"
)

// AutoMigrate creates or updates every table. (moved from internal/model)
func AutoMigrate(g *gorm.DB) error {
	return g.AutoMigrate(
		// ... exact list moved from model.AutoMigrate ...
	)
}

// BackfillRecipePhotos ... (moved verbatim from internal/model)
func BackfillRecipePhotos(g *gorm.DB) error {
	// ... exact body moved from model.BackfillRecipePhotos ...
}
```

- [ ] **Step 4: Update all call sites.** Replace across the repo, then fix imports:

```bash
grep -rl "model.AutoMigrate\|model.BackfillRecipePhotos" --include=*.go internal cmd \
  | xargs sed -i 's/model\.AutoMigrate/db.AutoMigrate/g; s/model\.BackfillRecipePhotos/db.BackfillRecipePhotos/g'
goimports -w internal cmd
```

Note: `internal/db/dialector_test.go` is `package db` — after replacement its calls are `AutoMigrate` in-package; adjust `db.AutoMigrate` → `AutoMigrate` there if `goimports` flags a self-import.

- [ ] **Step 5: Run the full suite and the guard**

Run: `rtk go test ./...` and `CGO_ENABLED=0 go build ./...`
Expected: PASS, build OK. `TestModelHasNoGormImport` passes.

- [ ] **Step 6: Commit**

```bash
rtk git add -A && rtk git commit -m "refactor(model): move AutoMigrate to internal/db; make model pure

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Turn `media.Store` into a port; rename the filesystem impl to `LocalStore`

**Files:**
- Modify: `internal/media/store.go`
- Create: `internal/media/port_test.go`
- Modify: `cmd/server/main.go:140` (`&media.Store{Dir: cfg.MediaDir}` → `media.NewLocalStore(cfg.MediaDir)`)
- Modify: `internal/router/router.go` (`Deps.Media` type → `media.Store`)
- Modify: `internal/herb/handler.go`, `internal/doctor/handler.go`, `internal/recipe/handler.go`, `internal/caserec/handler.go` (change every `*media.Store` parameter to `media.Store`)

**Interfaces:**
- Produces:
  - `media.Store` interface: `SaveReader(io.Reader) (string, error)`, `SaveMultipart(*multipart.FileHeader) (string, error)`, `UsageBytes() (int64, error)`.
  - `media.LocalStore` struct (was `Store`) with field `Dir string`; `media.NewLocalStore(dir string) *LocalStore`.
- Consumed by: herb (`UsageBytes`), doctor/recipe/caserec (`SaveMultipart`).

- [ ] **Step 1: Write the failing test** — `internal/media/port_test.go`:

```go
package media

import (
	"io"
	"mime/multipart"
	"testing"
)

// assert the local filesystem adapter satisfies the port.
var _ Store = (*LocalStore)(nil)

func TestStoreIsInterface(t *testing.T) {
	var s Store = NewLocalStore(t.TempDir())
	if _, err := s.UsageBytes(); err != nil {
		t.Fatalf("UsageBytes on empty dir: %v", err)
	}
	// compile-time proof the port shape matches usage:
	var _ func(io.Reader) (string, error) = s.SaveReader
	var _ func(*multipart.FileHeader) (string, error) = s.SaveMultipart
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/media/`
Expected: FAIL to compile (`Store` is a struct, `LocalStore`/`NewLocalStore` undefined).

- [ ] **Step 3: Implement the port.** In `store.go`: add the `Store` interface; rename `type Store struct` → `type LocalStore struct`; change every method receiver `(s *Store)` → `(s *LocalStore)`; add `NewLocalStore`.

```go
// Store saves and measures uploaded images. LocalStore is the filesystem
// adapter; sub-project B adds an S3 (Garage) adapter behind the same port.
type Store interface {
	SaveReader(r io.Reader) (string, error)
	SaveMultipart(fh *multipart.FileHeader) (string, error)
	UsageBytes() (int64, error)
}

// LocalStore stores images under Dir.
type LocalStore struct{ Dir string }

// NewLocalStore returns a filesystem-backed Store rooted at dir.
func NewLocalStore(dir string) *LocalStore { return &LocalStore{Dir: dir} }
```

- [ ] **Step 4: Update consumers.** In the 4 handlers change `mediaStore *media.Store` → `mediaStore media.Store` (RegisterRoutes and the `photoHandler`/`storageHandler` closures). In `router.go` change `Media *media.Store` → `Media media.Store`. In `main.go` change the construction to `media.NewLocalStore(cfg.MediaDir)`. Run `goimports -w`.

- [ ] **Step 5: Run**

Run: `rtk go test ./...` and `CGO_ENABLED=0 go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
rtk git add -A && rtk git commit -m "refactor(media): extract Store port, rename impl to LocalStore

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Tasks 3–11: Feature repository ports (one task each)

Every one of these tasks follows the identical shape below. Only the package name, the interface method set, and the `RegisterRoutes` signature differ — each is given in full so no task references another.

**Per-task shape (apply for the package named in the task):**

- [ ] **Step 1:** Create `internal/<pkg>/port_test.go` (internal, `package <pkg>`):

```go
package <pkg>

// assert the GORM adapter satisfies the handler's port.
var _ repository = (*Repo)(nil)
```

- [ ] **Step 2:** Run `rtk go test ./internal/<pkg>/` — expect FAIL to compile (`repository` undefined).
- [ ] **Step 3:** In `handler.go` add the `repository` interface (below) and change `RegisterRoutes` (and any handler closures that take `repo *Repo`) to take `repo repository`.
- [ ] **Step 4:** Run `rtk go test ./internal/<pkg>/ ./internal/router/` — expect PASS (the concrete `*Repo` passed by `router` still satisfies the interface).
- [ ] **Step 5:** Commit `refactor(<pkg>): depend on repository port, not *Repo`.

The interfaces (copy verbatim into the named package's `handler.go`):

**Task 3 — `herb`** (`RegisterRoutes(r gin.IRouter, repo repository, mediaStore media.Store)`):
```go
type repository interface {
	List() ([]model.Herb, error)
	Get(id uint) (model.Herb, error)
	Create(h *model.Herb, createdByDistrictID *uint) error
	Update(h *model.Herb, editorDistrictID *uint) error
	Delete(id uint) error
	PendingNames() ([]string, error)
	Reconcile(pendingName string, herbID uint) (int64, error)
	Merge(aliasID, canonicalID uint) (int64, error)
	NearDuplicates(thaiName string) ([]model.Herb, error)
}
```

**Task 4 — `district`** (`RegisterRoutes(r gin.IRouter, repo repository)`):
```go
type repository interface {
	List() ([]model.District, error)
	Get(id uint) (model.District, error)
	Create(d *model.District) error
	Update(d *model.District) error
	Delete(id uint) error
}
```

**Task 5 — `user`** (`RegisterRoutes(r gin.IRouter, repo repository)`):
```go
type repository interface {
	List() ([]model.User, error)
	Get(id uint) (model.User, error)
	Create(u *model.User) error
	Update(u *model.User) error
	SetActive(id uint, active bool) error
	SetPassword(id uint, hash string) error
}
```

**Task 6 — `yearlock`** (`RegisterRoutes(r gin.IRouter, repo repository)`):
```go
type repository interface {
	IsLocked(dataYear int) (bool, error)
	Lock(dataYear int, actorID uint) error
	Unlock(dataYear int) error
	List() ([]model.YearLock, error)
}
```

**Task 7 — `doctor`** (`RegisterRoutes(r gin.IRouter, repo repository, mediaStore media.Store)`; also change `photoHandler(repo repository, ...)`):
```go
type repository interface {
	ListByDistrict(districtID uint) ([]model.Doctor, error)
	Get(id uint) (model.Doctor, error)
	Create(d *model.Doctor, actorID uint, immediate bool) error
	Update(d *model.Doctor, actorID uint, immediate bool) error
	Delete(id, actorID uint, immediate bool) error
	SetPhoto(id, actorID uint, path string, immediate bool) error
	Unpublish(id uint) error
}
```

**Task 8 — `recipe`** (`RegisterRoutes(r gin.IRouter, repo repository, mediaStore media.Store)`; also `photoHandler(repo repository, ...)`):
```go
type repository interface {
	ListByDoctor(doctorID uint) ([]model.Recipe, error)
	GetIngredients(recipeID uint) ([]model.Ingredient, error)
	Get(id uint) (model.Recipe, error)
	Create(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error
	Update(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error
	AddPhoto(id uint, path string) error
	GetPhotos(recipeID uint) ([]model.RecipePhoto, error)
	Delete(id, actorID uint, immediate bool) error
	ResolveDoctor(code, nameForCheck string) (doctorID uint, mismatch bool, err error)
}
```

**Task 9 — `caserec`** (`RegisterRoutes(r gin.IRouter, repo repository, mediaStore media.Store)`; also `photoHandler(repo repository, ...)`):
```go
type repository interface {
	ListByRecipe(recipeID uint) ([]model.Case, error)
	Get(id uint) (model.Case, error)
	Create(c *model.Case, actorID uint, immediate bool) error
	Update(c *model.Case, actorID uint, immediate bool) error
	Delete(id, actorID uint, immediate bool) error
	SetPhoto(id, actorID uint, path string, immediate bool) error
	DistrictOf(recipeID uint) (uint, error)
}
```

**Task 10 — `review`** (`RegisterRoutes(r gin.IRouter, repo repository)`):
```go
type repository interface {
	Queue(districtID *uint) ([]Item, error)
	Approve(entityType string, entityID, actorID uint) error
	Reject(entityType string, entityID, actorID uint, reason string) error
	ApproveDoctorTree(doctorID, actorID uint) (int, error)
	Detail(entityType string, id uint) (Detail, error)
}
```

**Task 11 — `importer`** (`RegisterRoutes(r gin.IRouter, im importerService)` — name the interface `importerService` to avoid clashing with the package name):
```go
type importerService interface {
	DryRun(r io.Reader, sourceName string) (*Report, error)
	Run(r io.Reader, sourceName string, actorID uint) (*Report, error)
	Undo(batchID uint) error
}
```
For Task 11, the `port_test.go` assertion is `var _ importerService = (*Importer)(nil)` and the router already passes `*Importer`.

---

### Task 12: `publicapi` — pass a repository port, build the repo in the composition root

**Files:**
- Modify: `internal/publicapi/handler.go` (`RegisterRoutes(r gin.IRouter, repo repository)`; remove the internal `NewRepo(g)` call)
- Modify: `internal/router/router.go:89` (`publicapi.RegisterRoutes(api, publicapi.NewRepo(deps.DB))`)
- Create: `internal/publicapi/port_test.go`

**Interfaces:**
- Produces: unexported `repository` interface (below); the existing exported `NewRepo`/`Repo` are unchanged and satisfy it.

- [ ] **Step 1:** `internal/publicapi/port_test.go`:
```go
package publicapi

var _ repository = (*Repo)(nil)
```
- [ ] **Step 2:** Run `rtk go test ./internal/publicapi/` — expect FAIL (`repository` undefined).
- [ ] **Step 3:** Add to `handler.go`:
```go
type repository interface {
	ListDoctors(f DoctorFilter) ([]Doctor, error)
	GetDoctor(id uint) (Doctor, error)
	ListRecipes(f RecipeFilter) ([]Recipe, error)
	ListRecipesByDoctor(doctorID uint) ([]Recipe, error)
	ListPhotosByRecipe(recipeID uint) ([]string, error)
	ListIngredientsByRecipe(recipeID uint) ([]PublicIngredient, error)
	ListCasesByRecipe(recipeID uint) ([]Case, error)
	ListHerbs() ([]Herb, error)
	ListDistricts() ([]District, error)
}
```
Change `RegisterRoutes(r gin.IRouter, g *gorm.DB)` → `RegisterRoutes(r gin.IRouter, repo repository)` and delete the `repo := NewRepo(g)` line (the closures already use `repo`). Update `router.go` to `publicapi.RegisterRoutes(api, publicapi.NewRepo(deps.DB))`. `goimports -w`.
- [ ] **Step 4:** Run `rtk go test ./internal/publicapi/ ./internal/router/ ./...` — expect PASS.
- [ ] **Step 5:** Commit `refactor(publicapi): depend on repository port; build repo at composition root`.

---

### Task 13: `export` — introduce a `Source` port over the CSV/XLSX writers

**Files:**
- Modify: `internal/export/handler.go` (add `Source` interface + `gormSource` adapter; `RegisterRoutes(r gin.IRouter, src Source)`)
- Modify: `internal/router/router.go:90` (`export.RegisterRoutes(api, export.NewSource(deps.DB))`)
- Create: `internal/export/port_test.go`

**Interfaces:**
- Produces: `export.Source` interface, `export.NewSource(g *gorm.DB) Source`.
- Consumes: the existing package-level `Doctors`/`Recipes`/`Cases` `exportFunc` writers (unchanged).

- [ ] **Step 1:** `internal/export/port_test.go`:
```go
package export

import "testing"

func TestGormSourceSatisfiesSource(t *testing.T) {
	var _ Source = gormSource{}
}
```
- [ ] **Step 2:** Run `rtk go test ./internal/export/` — expect FAIL (`Source`, `gormSource` undefined).
- [ ] **Step 3:** In `handler.go` add:
```go
// Source writes an export stream for one entity in the given format, scoped to
// districtID (nil = all districts).
type Source interface {
	Doctors(w io.Writer, format string, districtID *uint) error
	Recipes(w io.Writer, format string, districtID *uint) error
	Cases(w io.Writer, format string, districtID *uint) error
}

type gormSource struct{ g *gorm.DB }

// NewSource returns a GORM-backed export Source.
func NewSource(g *gorm.DB) Source { return gormSource{g: g} }

func (s gormSource) Doctors(w io.Writer, format string, d *uint) error { return Doctors(w, s.g, format, d) }
func (s gormSource) Recipes(w io.Writer, format string, d *uint) error { return Recipes(w, s.g, format, d) }
func (s gormSource) Cases(w io.Writer, format string, d *uint) error   { return Cases(w, s.g, format, d) }
```
Change `RegisterRoutes(r gin.IRouter, g *gorm.DB)` → `RegisterRoutes(r gin.IRouter, src Source)`. Replace the `{name, fn}` loop so each route calls the matching `src` method instead of `exportHandler(g, ...)`; change `exportHandler` to close over a `func(io.Writer, string, *uint) error` (bound `src` method) rather than `(g, fn)`. Update `router.go`. `goimports -w`.

Note: verify the exact `exportFunc` type signature in `export.go` and adapt the three method bodies to match it (the plan assumes `func(w io.Writer, g *gorm.DB, format string, districtID *uint) error`).

- [ ] **Step 4:** Run `rtk go test ./internal/export/ ./...` — expect PASS.
- [ ] **Step 5:** Commit `refactor(export): route handlers through a Source port`.

---

### Task 14: `auth` — Sessions, Limiter, and Users ports

**Files:**
- Modify: `internal/auth/session.go` (add `Sessions` interface)
- Modify: `internal/auth/throttle.go` (add `Limiter` interface)
- Modify: `internal/auth/middleware.go` (`LoadUser(store Sessions, users Users)`)
- Modify: `internal/auth/handler.go` (add `Users` interface + `gormUsers` adapter; `RegisterRoutes(r gin.IRouter, users Users, store Sessions, th Limiter, secure bool)`)
- Modify: `internal/router/router.go` (build `auth.NewGormUsers(deps.DB)`; `Deps.Store` → `auth.Sessions`, `Deps.Throttle` → `auth.Limiter`)
- Modify: `cmd/server/main.go` (no type change needed — `*SessionStore`/`*Throttle` satisfy the interfaces)
- Create: `internal/auth/port_test.go`

**Interfaces:**
- Produces:
  - `Sessions`: `Create(userID uint) (string, error)`, `Lookup(rawToken string) (uint, error)`, `Delete(rawToken string) error`.
  - `Limiter`: `Allowed(key string) bool`, `Fail(key string)`, `Reset(key string)`.
  - `Users`: `ByActiveEmail(email string) (model.User, error)`, `ByID(id uint) (model.User, error)`; `NewGormUsers(g *gorm.DB) Users`.

- [ ] **Step 1:** `internal/auth/port_test.go`:
```go
package auth

var (
	_ Sessions = (*SessionStore)(nil)
	_ Limiter  = (*Throttle)(nil)
	_ Users    = gormUsers{}
)
```
- [ ] **Step 2:** Run `rtk go test ./internal/auth/` — expect FAIL (interfaces + `gormUsers` undefined).
- [ ] **Step 3:** Add the three interfaces in their files. In `handler.go` add the adapter and re-point the login query through it:
```go
type Users interface {
	ByActiveEmail(email string) (model.User, error)
	ByID(id uint) (model.User, error)
}

type gormUsers struct{ g *gorm.DB }

// NewGormUsers returns a GORM-backed Users port.
func NewGormUsers(g *gorm.DB) Users { return gormUsers{g: g} }

func (u gormUsers) ByActiveEmail(email string) (model.User, error) {
	var user model.User
	err := u.g.Where("email = ? AND active = ?", email, true).First(&user).Error
	return user, err
}

func (u gormUsers) ByID(id uint) (model.User, error) {
	var user model.User
	err := u.g.First(&user, id).Error
	return user, err
}
```
Change the login handler to call `users.ByActiveEmail(req.Email)` instead of the inline `g.Where(...)`. Change `LoadUser(store *SessionStore, g *gorm.DB)` → `LoadUser(store Sessions, users Users)` and replace its inline user-by-id load with `users.ByID(...)` (match the existing not-found handling). Change `RegisterRoutes` signature as above. Update `router.go`: build `authUsers := auth.NewGormUsers(deps.DB)`, pass it to both `auth.LoadUser(deps.Store, authUsers)` and `auth.RegisterRoutes(api, authUsers, deps.Store, deps.Throttle, deps.Secure)`; change the `Deps.Store`/`Deps.Throttle` field types to `auth.Sessions`/`auth.Limiter`. `goimports -w`.

Note: read `middleware.go:27-68` first to match the exact current not-found / inactive-user branch, so behavior is identical.

- [ ] **Step 4:** Run `rtk go test ./internal/auth/ ./internal/router/ ./...` — expect PASS (all auth/middleware/throttle tests green).
- [ ] **Step 5:** Commit `refactor(auth): Sessions, Limiter, Users ports; invert login and LoadUser`.

---

### Task 15: Verify success criteria, update CONTEXT.md, integrate

**Files:**
- Modify: `CONTEXT.md`

- [ ] **Step 1: Add the guard test** confirming no handler imports a DB driver — `internal/router/deps_ports_test.go`:
```go
package router

import "testing"

// Deps must expose the swappable seams as interfaces so sub-projects B and C
// can inject alternate adapters. This is a compile-time guarantee.
func TestDepsSeamsAreInterfaces(t *testing.T) {
	var d Deps
	_ = d // fields Media/Store/Throttle are interface types; see field decls.
}
```
- [ ] **Step 2: Run the whole verification**

```bash
rtk go test ./...
CGO_ENABLED=0 go build ./...
grep -n "gorm" internal/model/model.go || echo "model is gorm-free ✅"
grep -rlnE "\*gorm\.DB|repo \*Repo|g \*gorm\.DB" internal/*/handler.go || echo "no handler depends on *gorm.DB/*Repo ✅"
```
Expected: all tests pass; both grep guards print the ✅ line (no matches).

- [ ] **Step 3: Update `CONTEXT.md`** — add an entry under the current status describing the hexagonal core refactor (sub-project A complete: model pure, repository/media/auth ports introduced, no behavior change). Keep to the file's existing format.

- [ ] **Step 4: Commit**

```bash
rtk git add -A && rtk git commit -m "refactor: complete hexagonal core (sub-project A); update CONTEXT.md

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 5: Integrate** — follow superpowers:finishing-a-development-branch (merge `refactor/hexagonal-core` to `main` or open a PR, per user preference).

---

## Self-Review

**Spec coverage:**
- "model imports only stdlib" → Task 1 (+ guard test).
- "no handler depends on *gorm.DB or concrete *Repo" → Tasks 3–14 (+ Task 15 grep guard).
- "Deps media/throttle/session are interfaces" → Task 2 (media), Task 14 (store/throttle).
- MediaStore/SessionStore/Throttle ports for B/C → Task 2, Task 14.
- publicapi/export take `*gorm.DB` (spec note) → Tasks 12, 13 handle them explicitly.
- No behavior/API change → every task runs the full suite; no route strings touched.

**Placeholder scan:** Interfaces are given verbatim per package. Two tasks carry explicit "read the exact current code first" notes (export `exportFunc` signature; auth `LoadUser` not-found branch) because those bodies must be matched, not invented — these are verification steps, not placeholders.

**Type consistency:** `repository` is the unexported port name in every feature package; `importerService` is used for importer to avoid the package-name clash; `media.Store` (interface) / `media.LocalStore` (impl) are used consistently in Tasks 2, 3, 7, 8, 9; `auth.Sessions`/`auth.Limiter`/`auth.Users` are consistent across Tasks 14 and 15.
