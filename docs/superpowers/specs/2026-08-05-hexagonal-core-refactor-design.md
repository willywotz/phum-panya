# Sub-project A — Hexagonal core refactor (design)

Date: 2026-08-05
Branch: `refactor/hexagonal-core`
Status: approved design, ready for implementation plan

## Program context

This spec is sub-project **A** of a larger program to make phum-panya fully
compliant with the 15-Factor App and Hexagonal architecture methodologies. The
program is decomposed into independent sub-projects, each with its own
spec → plan → build cycle:

| # | Sub-project | Delivers | Depends on |
|---|---|---|---|
| **A** | Hexagonal core refactor | Port interfaces for every driven side; domain core imports no infrastructure. Pure refactor, no behavior change. | — |
| **B** | Media → object storage | `MediaStore` port + S3 adapter; default backend **Garage** (compose service). | A |
| **C** | Postgres-only + shared state | Drop SQLite path; throttle moves to a Postgres `UNLOGGED` table behind a `Throttle` port; sessions already shared. | A |
| **D** | Telemetry | Structured logs, metrics endpoint, request tracing. | A |
| **E** | Migrations as release step + multi-replica | Migrations run as a discrete job, not at startup; Caddy load-balances N `api` replicas. | B, C |

Recommended order: **A → B → C → D → E**. A is the keystone: the ports it
defines are what B/C/D plug into.

### Locked decisions (apply to later sub-projects)

- **Object store (B):** Garage, self-hosted, S3-compatible. One `MediaStore`
  port, one S3 adapter via `aws-sdk-go-v2`, endpoint + credentials from env.
  Cloud S3 stays possible by env change only, no code change.
- **Throttle store (C):** Postgres `UNLOGGED` table behind a `Throttle` port.
  No WAL overhead; losing counters on a crash is harmless. Postgres-specific,
  which is fine because C drops SQLite.

## 1. Goal and non-goals

**Goal:** Invert the dependencies so the domain core imports no infrastructure,
and add the ports that B, C, and D plug into.

**This is a pure internal refactor.** No behavior change. No API change. No new
dependency. All existing tests (222 at design time) stay green throughout.

**Non-goals (later sub-projects):**
- No directory reorganization (that was the rejected Approach 2).
- No entity duplication / mapping layer.
- No object store (B), no Postgres-only change (C), no telemetry (D).

## 2. Target dependency rule

- **Inward:** gin handlers (driving adapters) → **port interfaces** → domain
  entities (`internal/model`).
- **Outward:** GORM repos, media store, session store, and throttle become
  **driven adapters** that implement the ports.
- After A: `internal/model` imports only the standard library. No package that
  holds a business rule imports GORM or gin.

## 3. Package and interface layout

The current feature-package layout is kept (follow the existing pattern; no
directory move).

- **`internal/model` → pure.** Move `AutoMigrate` and `BackfillRecipePhotos`
  into a new `internal/db/migrate.go` (an adapter). The entity structs keep
  their GORM tags — these are inert strings and create no compile dependency —
  so `model` drops its `gorm.io/gorm` import and imports only `time`.

- **Per feature** (`herb`, `doctor`, `recipe`, `caserec`, `review`, `user`,
  `district`, `publicapi`, `export`, `yearlock`, `importer`, `backup`): the
  handler consumes a small **port interface** it defines (for example a
  `herbRepository` interface with `List`, `Get`, `Create`, `Update`, `Delete`,
  `Merge`, `PendingNames`, `Reconcile`, `NearDuplicates`). The existing GORM
  `Repo` implements it automatically (Go structural typing). `RegisterRoutes`
  takes the interface, not `*Repo`.

  Interfaces are defined at the consumer (idiomatic Go: "accept interfaces,
  return structs"), so no central port package explosion is needed for feature
  repos.

  Note: `publicapi` and `export` currently receive `*gorm.DB` directly (no
  `Repo`). Inverting them means introducing a repository adapter for each and a
  matching port interface — a slightly larger change than the packages that
  already have a `Repo`.

- **Shared infrastructure ports** (used by several features), defined where
  consumed and satisfied by the current concrete types as default adapters:
  - `MediaStore` — `SaveReader`, `SaveMultipart`, `UsageBytes`. Current
    `media.Store` (filesystem) is the default adapter. This seam is what B's S3
    (Garage) adapter later swaps in.
  - `SessionStore` — session lookup/create/delete used by `auth.LoadUser`.
  - `Throttle` — the login rate-limiter. This seam is what C's Postgres adapter
    swaps in.
  - `Clock` — already an interface (`internal/clock.Clock`); keep as is.

- **Composition root** stays `cmd/server/main.go` + `internal/router.NewEngine`.
  It constructs the concrete adapters and injects them as interfaces.

## 4. Composition and wiring

`router.Deps` already injects dependencies. Change its field types from concrete
(`*media.Store`, `*auth.Throttle`) to the port interfaces so `NewEngine` wires
ports. `main.go` constructs the concrete adapters and passes them in.
`NewEngine` is already the wiring seam, so the change is small and localized.

## 5. Testing (TDD, mandatory)

- The existing tests are the safety net. They must stay green after every step;
  run them (`rtk go test ./...`) after each feature's inversion.
- For each new port, follow red → green → refactor: write a handler test that
  uses a **fake adapter** first (proves the seam is an interface), confirm it
  fails, invert the wiring, confirm it passes, then refactor.
- Because behavior does not change, no other behavioral tests are added.

## 6. Rollout inside A (small commits, all tests green each time)

1. `model` purity — move `AutoMigrate` + `BackfillRecipePhotos` to
   `internal/db/migrate.go`; drop the GORM import from `model`.
2. One commit per feature package — invert `Repo` → interface at the handler /
   `RegisterRoutes` seam.
3. One commit — formalize the shared infrastructure ports (`MediaStore`,
   `SessionStore`, `Throttle`) as interface fields in `router.Deps`.

## 7. Branch and process

- Branch: `refactor/hexagonal-core` (never on `main`).
- TDD is mandatory for every step (project rule).
- When A is done: update `CONTEXT.md`, commit, then integrate.

## Success criteria

- `internal/model` imports only the standard library (verified: no `gorm` in
  its import list).
- No `internal/**/handler.go` depends on `*gorm.DB` or a concrete `*Repo`; each
  depends on a port interface.
- `router.Deps` media/throttle/session fields are interface types.
- All existing tests pass with no behavior or API change.
