# Migrations-as-release + Multi-replica (Sub-project E) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make schema migration a discrete release step (`server migrate` + a compose migrate job), gated at startup by `APP_AUTO_MIGRATE`, and run 2 api replicas behind Caddy load-balancing.

**Architecture:** Extract `migrateDB` (AutoMigrate + Backfill + EnsureAdmin); add a `migrate` subcommand; `runServer` migrates only when `APP_AUTO_MIGRATE=true` (default). Prod compose adds a one-shot `migrate` job, sets `deploy.replicas: 2` + `APP_AUTO_MIGRATE=false` on the api, and Caddy load-balances across replicas via dynamic A-record upstreams. Dev unchanged.

**Tech Stack:** Go 1.26, GORM, Docker Compose v2, Caddy.

## Global Constraints

- TDD mandatory: failing test → confirm fail → minimal code → confirm pass → refactor.
- No business/route change. Single-binary/dev keep startup auto-migrate (default). Dev stays single-replica.
- Existing suite (249 Go tests) stays green after every task. `rtk go test ./...`; build `CGO_ENABLED=0 go build ./...`; vet `go vet ./...`.
- Never commit to `main`; branch `feat/multi-replica` (already created).
- `Dockerfile.api` has `ENTRYPOINT ["server"]`, so a compose `command: ["migrate"]` runs `server migrate`.
- Uber Go style; American English; organized imports (`goimports`; if missing `go run golang.org/x/tools/cmd/goimports@latest -w <files>`).
- The Caddy dynamic-upstream / health-check syntax and `deploy.replicas` round-robin under `docker compose up` are verified LIVE by the orchestrator (Task 3 step 9) and adjusted if the exact directives differ by Caddy version. Builders produce the best-effort config + `caddy validate`.
- Builders do NOT run git or touch `CONTEXT.md`/docker state — the orchestrator commits after review and runs the live 2-replica check.

---

## File Structure

- `internal/config/config.go` — `AutoMigrate bool`.
- `cmd/server/main.go` — `migrateDB` helper, `migrate` subcommand (`runMigrate`), `runServer` gating.
- `cmd/server/main_migrate_test.go` — **new**: `migrateDB` test.
- `docker-compose.yaml` — `migrate` job; api `deploy.replicas: 2` + `APP_AUTO_MIGRATE=false` + depends_on migrate; admin env moved to migrate.
- `deploy/caddy/Caddyfile` — dynamic-upstream load-balancing for `/api/*` and `/media/*`.
- `.env.example` — one-line note.
- `docs/adr/0007-migrations-release-multireplica.md` — **new** ADR.

---

### Task 1: Config — `APP_AUTO_MIGRATE`

**Files:** Modify `internal/config/config.go`; Test `internal/config/config_test.go`.

**Interfaces:** Produces `Config.AutoMigrate bool`.

- [ ] **Step 1: Failing test** — append to `config_test.go`:

```go
func TestAutoMigrateDefaultsTrue(t *testing.T) {
	t.Setenv("APP_AUTO_MIGRATE", "")
	if !Load().AutoMigrate {
		t.Fatal("AutoMigrate default = false, want true")
	}
}

func TestAutoMigrateFalseWhenSet(t *testing.T) {
	t.Setenv("APP_AUTO_MIGRATE", "false")
	if Load().AutoMigrate {
		t.Fatal("AutoMigrate = true when APP_AUTO_MIGRATE=false")
	}
}
```

- [ ] **Step 2: Run — FAIL** (`rtk go test ./internal/config/`).
- [ ] **Step 3: Implement.** Add field + `Load()` line:

```go
	AutoMigrate bool
```
```go
	AutoMigrate: env("APP_AUTO_MIGRATE", "true") != "false",
```

- [ ] **Step 4: Run — PASS** (`rtk go test ./internal/config/ ./...`).
- [ ] **Step 5: Commit** — `feat(config): APP_AUTO_MIGRATE gate`.

---

### Task 2: `migrateDB` helper + `migrate` subcommand + gate `runServer`

**Files:** Modify `cmd/server/main.go`; Test `cmd/server/main_migrate_test.go`.

**Interfaces:**
- Consumes: `db.OpenWith`, `db.AutoMigrate`, `db.BackfillRecipePhotos`, `bootstrap.EnsureAdmin(g, email, password) (bool, error)`, `config.Config.AutoMigrate`.
- Produces: `migrateDB(g *gorm.DB, cfg config.Config) error`; `runMigrate()`; `migrate` os.Args case.

- [ ] **Step 1: Failing test** — `cmd/server/main_migrate_test.go` (`package main`):

```go
package main

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestMigrateDBCreatesSchemaAndSeedsAdmin(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	cfg := config.Config{AdminEmail: "admin@x", AdminPassword: "pw"}
	if err := migrateDB(g, cfg); err != nil {
		t.Fatalf("migrateDB: %v", err)
	}
	var u model.User
	if err := g.Where("email = ?", "admin@x").First(&u).Error; err != nil {
		t.Fatalf("admin not seeded: %v", err)
	}
	if u.Role != model.RoleCentralAdmin {
		t.Fatalf("admin role = %q, want central_admin", u.Role)
	}
}
```

- [ ] **Step 2: Run — FAIL** (`rtk go test ./cmd/server/`) — `migrateDB` undefined.
- [ ] **Step 3: Implement.** In `main.go` add the helper and the subcommand, and gate `runServer`.

Add the helper:

```go
// migrateDB runs the schema migration, the recipe-photo backfill, and the
// first-admin seed. It is the release step: the migrate job runs it once, and
// runServer runs it at startup only when APP_AUTO_MIGRATE is true.
func migrateDB(g *gorm.DB, cfg config.Config) error {
	if err := db.AutoMigrate(g); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := db.BackfillRecipePhotos(g); err != nil {
		return fmt.Errorf("backfill recipe photos: %w", err)
	}
	if _, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}
	return nil
}

// runMigrate is the `server migrate` subcommand: the discrete release step for
// the multi-replica stack.
func runMigrate() {
	cfg := config.Load()
	slog.SetDefault(telemetry.NewLogger(cfg))
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	if err := migrateDB(g, cfg); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("migrate: done")
}
```

Add the subcommand case in `main()` (beside `create-admin`):

```go
	case len(args) >= 1 && args[0] == "migrate":
		runMigrate()
```

In `runServer`, replace the three inline blocks (AutoMigrate, BackfillRecipePhotos, EnsureAdmin) with a single gated call:

```go
	if cfg.AutoMigrate {
		if err := migrateDB(g, cfg); err != nil {
			slog.Error("migrate", "err", err)
			os.Exit(1)
		}
	}
```

Confirm `fmt`, `gorm.io/gorm`, `phum-panya/internal/bootstrap`, `phum-panya/internal/db`, `phum-panya/internal/telemetry` are imported (most already are; `gorm` is needed for the `migrateDB` signature). Run `goimports -w cmd/server/main.go`.

- [ ] **Step 4: Run — PASS** (`rtk go test ./cmd/server/ ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`). The existing smoke test still migrates its own DB, so it is unaffected.
- [ ] **Step 5: Commit** — `feat(server): migrate subcommand; gate startup migrate on APP_AUTO_MIGRATE`.

---

### Task 3: Prod compose migrate job + 2 api replicas + Caddy load-balancing + docs

**Files:** Modify `docker-compose.yaml`, `deploy/caddy/Caddyfile`, `.env.example`; Create `docs/adr/0007-migrations-release-multireplica.md`.

- [ ] **Step 1: Add the `migrate` one-shot service** to `docker-compose.yaml` (api image; runs `server migrate` once). Place it near the api:

```yaml
  migrate:
    build:
      context: .
      dockerfile: Dockerfile.api
    depends_on:
      postgres:
        condition: service_healthy
    restart: on-failure
    command: ["migrate"]
    healthcheck:
      disable: true
    environment:
      APP_DB_DRIVER: postgres
      APP_DATABASE_URL: postgres://${POSTGRES_USER:-phumpanya}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB:-phumpanya}?sslmode=disable
      APP_ADMIN_EMAIL: ${APP_ADMIN_EMAIL:-admin@example.com}
      APP_ADMIN_PASSWORD: ${APP_ADMIN_PASSWORD:?set APP_ADMIN_PASSWORD}
```

- [ ] **Step 2: Make the api multi-replica.** In the `api` service:
  - Add:
    ```yaml
    deploy:
      replicas: 2
    ```
  - Add `APP_AUTO_MIGRATE: "false"` to its `environment`.
  - Remove `APP_ADMIN_EMAIL` and `APP_ADMIN_PASSWORD` from the api `environment` (the migrate job owns seeding now).
  - Add to `depends_on`:
    ```yaml
      migrate:
        condition: service_completed_successfully
    ```
    (keep the existing postgres/garage/garage-init conditions).

- [ ] **Step 3: Caddy load-balancing.** In `deploy/caddy/Caddyfile`, replace `reverse_proxy api:8080` inside BOTH the `handle /api/*` and `handle /media/*` blocks with a dynamic-upstream block that discovers the replicas and round-robins with an active health check:

```caddyfile
	handle /api/* {
		reverse_proxy {
			dynamic a {
				name api
				port 8080
				refresh 10s
			}
			lb_policy round_robin
			health_uri /api/health
			health_interval 10s
		}
	}

	handle /media/* {
		reverse_proxy {
			dynamic a {
				name api
				port 8080
				refresh 10s
			}
			lb_policy round_robin
			health_uri /api/health
			health_interval 10s
		}
	}
```

Leave the `/traces` `forward_auth` block's `api:8080` target as-is (single target for the auth sub-request is fine).

- [ ] **Step 4: `.env.example`** — add near the top-of-file usage notes:

```sh
# The prod stack runs DB migration as a one-shot `migrate` job before the api
# replicas start; the api runs with APP_AUTO_MIGRATE=false. The single binary
# leaves APP_AUTO_MIGRATE at its default (true) and migrates on startup.
```

- [ ] **Step 5: ADR-0007** — write `docs/adr/0007-migrations-release-multireplica.md` per the 0006 format: context (startup migrate races with N replicas; A–D removed per-process state so the api can scale), decision (config-gated startup migrate + `server migrate` subcommand + one-shot migrate job; api `deploy.replicas: 2`; Caddy dynamic-upstream round-robin with `/api/health` checks; dev stays single-replica), consequences (prod deploy has a migrate step; single-binary/dev unchanged; sessions/media/throttle already shared so replicas are safe).

- [ ] **Step 6: Validate Caddy** (needs `APP_DOMAIN`):
```bash
docker run --rm -e APP_DOMAIN=example.org -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1 | tail -3
```
Expected: "Valid configuration". If the `dynamic a` / `health_uri` directive names differ in the pinned Caddy version, adjust to the version's syntax until it validates, and note it.

- [ ] **Step 7: Validate compose**:
```bash
GARAGE_RPC_SECRET=x APP_S3_ACCESS_KEY=k APP_S3_SECRET_KEY=s APP_DOMAIN=example.org APP_ADMIN_PASSWORD=p POSTGRES_PASSWORD=pp docker compose -f docker-compose.yaml config -q && echo PROD_OK
```
Also confirm the api has `replicas: 2` and no admin env: `... docker compose -f docker-compose.yaml config | grep -E "replicas|APP_AUTO_MIGRATE|APP_ADMIN"`.

- [ ] **Step 8: Commit** — `feat(deploy): migrate job + 2 api replicas + Caddy load-balancing (ADR-0007)`.

- [ ] **Step 9 (orchestrator): live capstone check.** Bring up postgres + garage + garage-init + migrate + api (`--scale`/replicas 2) + caddy; confirm: (a) `migrate` exits 0 and the replicas do NOT migrate; (b) repeated requests through Caddy are served by BOTH replicas (distinct `instance` in JSON logs); (c) stopping one replica → requests still succeed (Caddy health-check failover). Then update `CONTEXT.md` and run the final whole-branch review.

---

## Self-Review

**Spec coverage:**
- `APP_AUTO_MIGRATE` config gate → Task 1.
- `migrateDB` helper + `migrate` subcommand + gated `runServer` → Task 2.
- Prod migrate job + api `replicas: 2` + `APP_AUTO_MIGRATE=false` + depends_on migrate + admin-env move → Task 3 (steps 1-2).
- Caddy dynamic-upstream load-balancing + health check → Task 3 (step 3), verified live (step 9).
- `.env.example` note + ADR-0007 → Task 3 (steps 4-5).
- Dev unchanged (no dev-compose task) — matches the spec's deliberate dev/prod difference.
- Single-binary unchanged (default `AutoMigrate=true`) — Task 1 default + Task 2 gate.
- Live capstone (migrate once, both replicas serve, failover) → Task 3 step 9.

**Placeholder scan:** The Caddy dynamic-upstream directive names and the ADR body are the only "verify against the real version / write per format" notes; both are deliberate (Caddy module syntax is version-specific and validated live; the ADR structure is specified). Every code/YAML block is complete.

**Type consistency:** `migrateDB(g *gorm.DB, cfg config.Config) error` is used identically in `runMigrate` and `runServer` (Task 2). `Config.AutoMigrate bool` (Task 1) is read in `runServer`'s gate (Task 2) and set as `APP_AUTO_MIGRATE` in compose (Task 3). `EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword)` matches the real signature `(g, email, password) (bool, error)`. The migrate service `command: ["migrate"]` matches the `migrate` os.Args case (Task 2) and the `ENTRYPOINT ["server"]` fact.
