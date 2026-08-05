# Compose stack deploy option (Postgres + Caddy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a second, optional deploy path — a Docker Compose stack of api + web + postgres + caddy plus a pg_dump sidecar — without changing the default single-binary SQLite path.

**Architecture:** The Go app gains a Postgres driver path and a "behind-proxy" serve mode, both behind config flags. New infra files (Dockerfiles, Caddyfile, compose, backup script) wire the four services. Caddy terminates TLS, serves `/media` reads directly, and reverse-proxies `/api/*` to the api and everything else to the web (Next.js standalone) service.

**Tech Stack:** Go 1.26 (GORM, Gin), `gorm.io/driver/postgres` (pure-Go pgx), Next.js 15 (standalone output), Caddy 2, PostgreSQL 17, Docker Compose.

## Global Constraints

- **TDD mandatory:** failing test → confirm fail → minimal code → confirm pass → refactor. No exceptions.
- **Go version floor:** `go 1.26.0` (module `phum-panya`).
- **cgo-free is a hard rule:** every Go build must pass `CGO_ENABLED=0 go build ./...`. Only pure-Go drivers.
- **Default path unchanged:** all new Go behaviour is behind config (`APP_DB_DRIVER`, `APP_BEHIND_PROXY`). With those unset, behaviour is byte-for-byte as before. Every existing test must stay green.
- **Style:** Uber Go style; American English names; organized imports sorted by path; minimal essential comments.
- **15-Factor + Hexagonal:** config via env only; no new coupling from domain packages to infra.
- **Design authority:** `docs/superpowers/specs/2026-08-05-compose-stack-deploy-design.md` and `docs/adr/0003-optional-compose-stack-postgres-caddy.md`.
- **Reference spec §4 media rule:** Caddy serves `GET`/`HEAD /media/*`; all other methods go to the api.

---

### Task 1: Config — new fields and deploy-mode predicates

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config` gains fields `DBDriver string`, `DatabaseURL string`, `BehindProxy bool`, `PublicOrigin string`. New methods `(Config) UseTLS() bool`, `(Config) CookieSecure() bool`, `(Config) AllowedOriginHost() string`, `(Config) BackupEnabled() bool`.

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"testing"
)

func TestModePredicates(t *testing.T) {
	cases := []struct {
		name        string
		cfg         Config
		wantTLS     bool
		wantSecure  bool
		wantHost    string
		wantBackup  bool
	}{
		{"dev", Config{DevMode: true}, false, false, "", true},
		{"prod-tls", Config{Domain: "example.org"}, true, true, "example.org", true},
		{"behind-proxy", Config{BehindProxy: true, PublicOrigin: "https://example.org"}, false, true, "example.org", true},
		{"postgres", Config{BehindProxy: true, PublicOrigin: "https://example.org", DBDriver: "postgres"}, false, true, "example.org", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.UseTLS(); got != tc.wantTLS {
				t.Errorf("UseTLS() = %v, want %v", got, tc.wantTLS)
			}
			if got := tc.cfg.CookieSecure(); got != tc.wantSecure {
				t.Errorf("CookieSecure() = %v, want %v", got, tc.wantSecure)
			}
			if got := tc.cfg.AllowedOriginHost(); got != tc.wantHost {
				t.Errorf("AllowedOriginHost() = %q, want %q", got, tc.wantHost)
			}
			if got := tc.cfg.BackupEnabled(); got != tc.wantBackup {
				t.Errorf("BackupEnabled() = %v, want %v", got, tc.wantBackup)
			}
		})
	}
}

func TestLoadNewDefaults(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.DBDriver != "sqlite" {
		t.Errorf("DBDriver default = %q, want sqlite", c.DBDriver)
	}
	if c.BehindProxy {
		t.Errorf("BehindProxy default = true, want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/config/ -run 'TestModePredicates|TestLoadNewDefaults' -v`
Expected: FAIL — undefined methods / fields.

- [ ] **Step 3: Write minimal implementation**

Add the imports and fields, and append the methods:

```go
// internal/config/config.go
package config

import (
	"net/url"
	"os"
)

type Config struct {
	HTTPAddr      string
	Domain        string
	DBDriver      string
	DBPath        string
	DatabaseURL   string
	MediaDir      string
	BackupDir     string
	DevMode       bool
	BehindProxy   bool
	PublicOrigin  string
	AdminEmail    string
	AdminPassword string
}

func Load() Config {
	return Config{
		HTTPAddr:      env("APP_HTTP_ADDR", ":8080"),
		Domain:        env("APP_DOMAIN", ""),
		DBDriver:      env("APP_DB_DRIVER", "sqlite"),
		DBPath:        env("APP_DB_PATH", "data/app.db"),
		DatabaseURL:   env("APP_DATABASE_URL", ""),
		MediaDir:      env("APP_MEDIA_DIR", "data/media"),
		BackupDir:     env("APP_BACKUP_DIR", "data/backup"),
		DevMode:       os.Getenv("APP_DEV") == "1",
		BehindProxy:   os.Getenv("APP_BEHIND_PROXY") == "1",
		PublicOrigin:  env("APP_PUBLIC_ORIGIN", ""),
		AdminEmail:    env("APP_ADMIN_EMAIL", ""),
		AdminPassword: env("APP_ADMIN_PASSWORD", ""),
	}
}

// UseTLS reports whether the server terminates TLS itself via autocert.
// False in dev, behind a reverse proxy, or when no domain is set.
func (c Config) UseTLS() bool {
	return !c.DevMode && !c.BehindProxy && c.Domain != ""
}

// CookieSecure reports whether the session cookie carries the Secure flag.
// True under production TLS and behind a proxy (public scheme is HTTPS).
func (c Config) CookieSecure() bool {
	return c.BehindProxy || (!c.DevMode && c.Domain != "")
}

// AllowedOriginHost returns the host the CSRF origin check must match.
// Behind a proxy it is the host of PublicOrigin; otherwise it is Domain.
func (c Config) AllowedOriginHost() string {
	if c.BehindProxy && c.PublicOrigin != "" {
		if u, err := url.Parse(c.PublicOrigin); err == nil {
			return u.Host
		}
	}
	return c.Domain
}

// BackupEnabled reports whether the in-app SQLite backup loop runs.
// It is disabled on Postgres, where the pg_dump sidecar owns backups.
func (c Config) BackupEnabled() bool {
	return c.DBDriver != "postgres"
}

func env(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
rtk git add internal/config/config.go internal/config/config_test.go
rtk git commit -m "feat(config): add postgres + behind-proxy fields and mode predicates"
```

---

### Task 2: DB driver selection (SQLite | Postgres)

**Files:**
- Modify: `internal/db/db.go`, `go.mod`, `go.sum`
- Modify: `cmd/server/main.go:120`, `cmd/server/main.go:191`
- Test: `internal/db/db_test.go`

**Interfaces:**
- Consumes: `config.Config.DBDriver`, `.DBPath`, `.DatabaseURL` (Task 1).
- Produces: `db.Dialector(driver, sqlitePath, pgDSN string) (gorm.Dialector, error)`, `db.OpenWith(driver, sqlitePath, pgDSN string) (*gorm.DB, error)`. `db.Open(path string)` stays as a SQLite wrapper.

- [ ] **Step 1: Add the pure-Go Postgres driver dependency**

Run: `rtk go get gorm.io/driver/postgres@latest`
Expected: `go.mod`/`go.sum` updated; no cgo pulled (pgx is pure Go).

- [ ] **Step 2: Write the failing test**

```go
// internal/db/db_test.go — add to the existing package db test file
package db

import (
	"os"
	"testing"

	"phum-panya/internal/model"
)

func TestDialectorSelectsDriver(t *testing.T) {
	sq, err := Dialector("sqlite", "x.db", "")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if sq.Name() != "sqlite" {
		t.Errorf("sqlite Name() = %q", sq.Name())
	}
	pg, err := Dialector("postgres", "", "postgres://u:p@h:5432/db")
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	if pg.Name() != "postgres" {
		t.Errorf("postgres Name() = %q", pg.Name())
	}
	if _, err := Dialector("mysql", "", ""); err == nil {
		t.Errorf("unknown driver: want error, got nil")
	}
}

// TestOpenWithPostgres round-trips against a live Postgres. It is skipped
// unless APP_DATABASE_URL is set (CI provides a postgres service).
func TestOpenWithPostgres(t *testing.T) {
	dsn := os.Getenv("APP_DATABASE_URL")
	if dsn == "" {
		t.Skip("APP_DATABASE_URL not set")
	}
	g, err := OpenWith("postgres", "", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	d := model.District{Name: "ทดสอบ"}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got model.District
	if err := g.First(&got, d.ID).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Name != "ทดสอบ" {
		t.Errorf("Name = %q", got.Name)
	}
}
```

> Confirm `model.District` has a `Name` field before running; if the district's display field differs, use that field name. Check with `rtk grep -n "type District" internal/model/*.go`.

- [ ] **Step 3: Run test to verify it fails**

Run: `rtk go test ./internal/db/ -run TestDialectorSelectsDriver -v`
Expected: FAIL — `Dialector` undefined.

- [ ] **Step 4: Write minimal implementation**

```go
// internal/db/db.go
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Dialector returns the GORM dialector for driver without connecting. For
// "sqlite" (or "") it wraps sqlitePath with the pure-Go glebarez driver and
// WAL pragmas; for "postgres" it uses pgDSN via the pure-Go pgx driver.
func Dialector(driver, sqlitePath, pgDSN string) (gorm.Dialector, error) {
	switch driver {
	case "", "sqlite":
		dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", sqlitePath)
		return sqlite.Open(dsn), nil
	case "postgres":
		return postgres.Open(pgDSN), nil
	default:
		return nil, fmt.Errorf("unknown db driver %q", driver)
	}
}

// OpenWith opens the database for driver and returns a configured *gorm.DB.
func OpenWith(driver, sqlitePath, pgDSN string) (*gorm.DB, error) {
	dial, err := Dialector(driver, sqlitePath, pgDSN)
	if err != nil {
		return nil, err
	}
	g, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", driver, err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	return g, nil
}

// Open opens a SQLite database at path (back-compat wrapper over OpenWith).
func Open(path string) (*gorm.DB, error) {
	return OpenWith("sqlite", path, "")
}

// Tx runs fn inside a transaction. Every query in fn MUST use tx.
func Tx(g *gorm.DB, fn func(tx *gorm.DB) error) error {
	return g.Transaction(fn)
}
```

- [ ] **Step 5: Wire main.go to the driver selection**

In `cmd/server/main.go`, replace both `db.Open(cfg.DBPath)` calls (lines 120 and 191):

```go
g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
```

- [ ] **Step 6: Run tests + cgo-free build**

Run: `rtk go test ./internal/db/ -v && CGO_ENABLED=0 rtk go build ./...`
Expected: PASS (Postgres round-trip SKIPPED locally); build clean.

- [ ] **Step 7: Commit**

```bash
rtk git add internal/db/db.go internal/db/db_test.go cmd/server/main.go go.mod go.sum
rtk git commit -m "feat(db): select sqlite or postgres driver from config"
```

---

### Task 3: Behind-proxy serve mode (no autocert behind Caddy)

**Files:**
- Modify: `internal/httpx/server.go:31-36`
- Test: `internal/httpx/server_test.go`

**Interfaces:**
- Consumes: `config.Config.UseTLS()` (Task 1).
- Produces: `ServeContext` serves plain HTTP whenever `!cfg.UseTLS()`, including behind-proxy mode.

- [ ] **Step 1: Write the failing test**

```go
// internal/httpx/server_test.go
package httpx_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"phum-panya/internal/config"
	"phum-panya/internal/httpx"
)

// TestServeContextBehindProxyServesPlainHTTP proves that with a domain set but
// BehindProxy on, the server serves plain HTTP on HTTPAddr and never tries ACME.
func TestServeContextBehindProxyServesPlainHTTP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg := config.Config{HTTPAddr: addr, Domain: "example.org", BehindProxy: true}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- httpx.ServeContext(ctx, cfg, h) }()

	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("serve returned %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/httpx/ -run TestServeContextBehindProxy -v`
Expected: FAIL — current code takes the TLS branch (Domain set, not dev) and never listens on `HTTPAddr`, so the GET times out.

- [ ] **Step 3: Write minimal implementation**

In `internal/httpx/server.go`, change `ServeContext` (line 31-36):

```go
func ServeContext(ctx context.Context, cfg config.Config, h http.Handler) error {
	if !cfg.UseTLS() {
		return serveHTTP(ctx, &http.Server{Addr: cfg.HTTPAddr, Handler: h})
	}
	return serveTLS(ctx, cfg, h)
}
```

Update the doc comment above it to mention behind-proxy mode.

- [ ] **Step 4: Run test to verify it passes**

Run: `rtk go test ./internal/httpx/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
rtk git add internal/httpx/server.go internal/httpx/server_test.go
rtk git commit -m "feat(httpx): serve plain HTTP behind a reverse proxy (no autocert)"
```

---

### Task 4: Secure cookie + CSRF origin host behind the proxy

**Files:**
- Modify: `cmd/server/main.go:142`, `internal/router/router.go:63`
- Test: `internal/router/router_test.go`

**Interfaces:**
- Consumes: `config.Config.CookieSecure()`, `.AllowedOriginHost()` (Task 1); `auth.SameOrigin(host string)` (existing).
- Produces: the global CSRF middleware matches the public origin under behind-proxy; the session cookie is Secure under behind-proxy.

- [ ] **Step 1: Write the failing test**

```go
// internal/router/router_test.go — add
// TestSameOriginUsesPublicOriginBehindProxy proves the CSRF origin check is
// wired to AllowedOriginHost, not Domain, so it is active behind a proxy.
func TestSameOriginUsesPublicOriginBehindProxy(t *testing.T) {
	mediaDir := t.TempDir()
	deps := testDeps(t, mediaDir) // existing helper; see newEngine in this file
	deps.Cfg = config.Config{BehindProxy: true, PublicOrigin: "https://example.org"}
	engine := router.NewEngine(deps)

	// A cross-origin unsafe request is rejected before routing.
	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST status = %d, want 403", rec.Code)
	}

	// A same-origin unsafe request is NOT rejected for origin (it proceeds to
	// the handler, which may 400/401 — anything but 403 forbidden_origin).
	req2 := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req2.Header.Set("Origin", "https://example.org")
	rec2 := httptest.NewRecorder()
	engine.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusForbidden {
		t.Fatalf("same-origin POST was forbidden by origin check")
	}
}
```

> Adapt to the existing test harness: this file already has `newEngine(t, mediaDir)` building `router.Deps`. Reuse that construction (rename the helper call as needed) and override `Cfg`. Confirm the unauthenticated POST route is `/api/login` with `rtk grep -rn "login" internal/auth/*.go`; if it differs, use the real path (any `/api/...` path works because the origin middleware runs before routing).

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/router/ -run TestSameOriginUsesPublicOrigin -v`
Expected: FAIL — router wires `SameOrigin(deps.Cfg.Domain)` (empty here), so the check is a no-op and the cross-origin POST is not 403.

- [ ] **Step 3: Write minimal implementation**

In `internal/router/router.go` line 63:

```go
engine.Use(auth.SameOrigin(deps.Cfg.AllowedOriginHost()))
```

In `cmd/server/main.go` line 142:

```go
Secure:     cfg.CookieSecure(),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `rtk go test ./internal/router/ ./internal/auth/ -v`
Expected: PASS; existing origin/session tests still green.

- [ ] **Step 5: Commit**

```bash
rtk git add internal/router/router.go cmd/server/main.go internal/router/router_test.go
rtk git commit -m "feat(router): enforce CSRF origin + secure cookie behind a proxy"
```

---

### Task 5: Skip the in-app SQLite backup on Postgres

**Files:**
- Modify: `cmd/server/main.go:150`, `internal/router/router.go:91`
- Test: `internal/router/router_test.go`

**Interfaces:**
- Consumes: `config.Config.BackupEnabled()` (Task 1).
- Produces: on Postgres the daily backup goroutine does not start and the manual backup route is not registered.

- [ ] **Step 1: Write the failing test**

```go
// internal/router/router_test.go — add
// TestBackupRouteAbsentOnPostgres proves the SQLite-only backup endpoint is not
// mounted when the app runs on Postgres (the pg_dump sidecar owns backups).
func TestBackupRouteAbsentOnPostgres(t *testing.T) {
	mediaDir := t.TempDir()
	deps := testDeps(t, mediaDir)
	deps.Cfg = config.Config{DBDriver: "postgres"}
	engine := router.NewEngine(deps)

	// Same-origin GET so the origin check passes; expect the route to be absent.
	req := httptest.NewRequest(http.MethodGet, "/api/backups", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/backups on postgres = %d, want 404", rec.Code)
	}
}
```

> Confirm the backup route path with `rtk grep -rn "backups\|backup" internal/backup/routes.go`; substitute the real registered path if it is not `/api/backups`.

- [ ] **Step 2: Run test to verify it fails**

Run: `rtk go test ./internal/router/ -run TestBackupRouteAbsentOnPostgres -v`
Expected: FAIL — route is registered unconditionally, so status is not 404.

- [ ] **Step 3: Write minimal implementation**

In `internal/router/router.go`, guard the registration (line 91):

```go
if deps.Cfg.BackupEnabled() {
	backup.RegisterRoutes(api, deps.DBPath, deps.MediaDir, deps.BackupDir, deps.BackupKeep, deps.Clk)
}
```

In `cmd/server/main.go`, guard the ticker (line 150):

```go
if cfg.BackupEnabled() {
	go runBackupTicker(cfg, clk)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `rtk go test ./internal/router/ -v && CGO_ENABLED=0 rtk go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
rtk git add internal/router/router.go cmd/server/main.go internal/router/router_test.go
rtk git commit -m "feat: disable in-app SQLite backup when running on postgres"
```

---

### Task 6: Next.js dual build mode (export | standalone)

**Files:**
- Modify: `web/next.config.mjs`

**Interfaces:**
- Produces: `NEXT_OUTPUT=standalone npm run build` emits `.next/standalone`; default `npm run build` emits `out/` (unchanged, for the embedded binary).

- [ ] **Step 1: Write the failing check**

Run: `cd web && NEXT_OUTPUT=standalone npm run build && test -f .next/standalone/server.js && echo OK`
Expected: FAIL — current config is `output: 'export'`, so no `.next/standalone/server.js` is produced.

- [ ] **Step 2: Implement the env-gated config**

```js
// web/next.config.mjs
/** @type {import('next').NextConfig} */
const standalone = process.env.NEXT_OUTPUT === 'standalone';

const nextConfig = {
  output: standalone ? 'standalone' : 'export',
  images: { unoptimized: true },
};

export default nextConfig;
```

- [ ] **Step 3: Verify both modes build**

Run:
```bash
cd web
NEXT_OUTPUT=standalone npm run build && test -f .next/standalone/server.js && echo STANDALONE_OK
npm run build && test -f out/index.html && echo EXPORT_OK
```
Expected: both `STANDALONE_OK` and `EXPORT_OK` print.

- [ ] **Step 4: Commit**

```bash
rtk git add web/next.config.mjs
rtk git commit -m "feat(web): env-gated standalone build mode alongside static export"
```

---

### Task 7: web service image (Next.js standalone)

**Files:**
- Create: `web/Dockerfile`
- Create: `web/.dockerignore`

**Interfaces:**
- Consumes: Task 6's `NEXT_OUTPUT=standalone` build.
- Produces: an image that runs `node server.js` on `PORT` (default 3000).

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# web/Dockerfile — Next.js standalone runtime for the compose stack.
# syntax=docker/dockerfile:1
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ENV NEXT_OUTPUT=standalone
RUN npm run build

FROM node:24-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
# standalone output bundles a minimal server + only the needed node_modules.
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
EXPOSE 3000
USER node
CMD ["node", "server.js"]
```

```
# web/.dockerignore
node_modules
.next
out
```

> If `web/public` does not exist, drop that COPY line (check with `ls web/public`).

- [ ] **Step 2: Verify the image builds**

Run: `docker build -f web/Dockerfile -t phum-panya-web:test web`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
rtk git add web/Dockerfile web/.dockerignore
rtk git commit -m "feat(web): Dockerfile for the standalone web service"
```

---

### Task 8: api service image (slim, API-only)

**Files:**
- Create: `Dockerfile.api`

**Interfaces:**
- Produces: a cgo-free Go image that embeds the committed `internal/webui/dist/` (no Node build stage). Serves on `:8080`.

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# Dockerfile.api — API-only image for the compose stack. Caddy serves the UI
# from the web service, so this image needs no Node build; it embeds the
# committed internal/webui/dist placeholder export.
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app \
 && mkdir -p /data && chown -R app:app /data
COPY --from=build /server /usr/local/bin/server
WORKDIR /
ENV APP_MEDIA_DIR=/data/media
VOLUME ["/data"]
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["server"]
```

> Confirm `.dockerignore` does not exclude `internal/webui/dist/` (the api image relies on the committed export). Check with `rtk grep -n "dist" .dockerignore`; if excluded, add `!internal/webui/dist` so it is kept.

- [ ] **Step 2: Verify the image builds**

Run: `docker build -f Dockerfile.api -t phum-panya-api:test .`
Expected: build succeeds; no Node stage.

- [ ] **Step 3: Commit**

```bash
rtk git add Dockerfile.api
rtk git commit -m "feat: slim API-only image for the compose stack"
```

---

### Task 9: Caddyfile (reverse proxy + media file_server)

**Files:**
- Create: `deploy/caddy/Caddyfile`

**Interfaces:**
- Produces: routing per spec §3 — `/api/*` and non-read `/media/*` → api:8080; `GET|HEAD /media/*` → file_server; everything else → web:3000. Automatic HTTPS for `{$APP_DOMAIN}`.

- [ ] **Step 1: Write the Caddyfile**

```caddyfile
# deploy/caddy/Caddyfile — one public origin; TLS via Caddy's ACME.
{$APP_DOMAIN} {
	encode gzip

	# Media reads are static files off the shared volume; writes go to the api.
	@media_read {
		method GET HEAD
		path /media/*
	}
	handle @media_read {
		root * /srv/media
		uri strip_prefix /media
		file_server
	}

	# API (and any non-read /media method, e.g. uploads) → the Go service.
	handle /api/* {
		reverse_proxy api:8080
	}
	handle /media/* {
		reverse_proxy api:8080
	}

	# Everything else → the Next.js standalone service.
	handle {
		reverse_proxy web:3000
	}
}
```

> `/srv/media` is where the shared media volume mounts read-only in the caddy container (Task 11). If uploads are served under a different prefix than `/media`, adjust the matchers to the real upload path found in Task 4's grep.

- [ ] **Step 2: Verify the Caddyfile is valid**

Run: `docker run --rm -e APP_DOMAIN=example.org -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile`
Expected: "Valid configuration".

- [ ] **Step 3: Commit**

```bash
rtk git add deploy/caddy/Caddyfile
rtk git commit -m "feat(deploy): Caddyfile routing api, media file_server, and web"
```

---

### Task 10: pg_dump backup sidecar script

**Files:**
- Create: `deploy/backup/backup.sh`

**Interfaces:**
- Produces: a loop that runs `pg_dump` daily into `/backups`, keeping the newest 14 dumps.

- [ ] **Step 1: Write the script**

```sh
#!/bin/sh
# deploy/backup/backup.sh — daily pg_dump into /backups, keep newest 14.
# Env: PGHOST, PGUSER, PGPASSWORD, PGDATABASE (standard libpq vars).
set -eu

OUT_DIR="${BACKUP_DIR:-/backups}"
KEEP="${BACKUP_KEEP:-14}"
mkdir -p "$OUT_DIR"

while true; do
	stamp="$(date +%Y-%m-%d)"
	file="$OUT_DIR/backup-$stamp.sql.gz"
	if pg_dump | gzip -c > "$file.tmp"; then
		mv "$file.tmp" "$file"
		echo "backup: wrote $file"
	else
		rm -f "$file.tmp"
		echo "backup: pg_dump failed" >&2
	fi
	# Prune: keep the newest $KEEP dumps.
	ls -1t "$OUT_DIR"/backup-*.sql.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | while read -r old; do
		rm -f "$old"
		echo "backup: pruned $old"
	done
	sleep 86400
done
```

- [ ] **Step 2: Verify the script parses**

Run: `sh -n deploy/backup/backup.sh && echo SYNTAX_OK`
Expected: `SYNTAX_OK`.

- [ ] **Step 3: Commit**

```bash
rtk git add deploy/backup/backup.sh
rtk git commit -m "feat(deploy): pg_dump backup sidecar script (keep newest 14)"
```

---

### Task 11: docker-compose.stack.yaml + .env.example

**Files:**
- Create: `docker-compose.stack.yaml`
- Modify: `.env.example`

**Interfaces:**
- Consumes: `Dockerfile.api`, `web/Dockerfile`, `deploy/caddy/Caddyfile`, `deploy/backup/backup.sh`.
- Produces: a 5-container stack (caddy, web, api, postgres, backup) with named volumes; only caddy publishes ports.

- [ ] **Step 1: Write the compose file**

```yaml
# docker-compose.stack.yaml — optional container stack (ADR-0003).
# Usage: set APP_DOMAIN + secrets in .env, then:
#   docker compose -f docker-compose.stack.yaml up -d --build
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-phumpanya}
      POSTGRES_USER: ${POSTGRES_USER:-phumpanya}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}
    volumes:
      - pg-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-phumpanya}"]
      interval: 10s
      timeout: 5s
      retries: 5

  api:
    build:
      context: .
      dockerfile: Dockerfile.api
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      APP_DB_DRIVER: postgres
      APP_DATABASE_URL: postgres://${POSTGRES_USER:-phumpanya}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB:-phumpanya}?sslmode=disable
      APP_BEHIND_PROXY: "1"
      APP_PUBLIC_ORIGIN: https://${APP_DOMAIN:?set APP_DOMAIN}
      APP_HTTP_ADDR: ":8080"
      APP_MEDIA_DIR: /data/media
      APP_ADMIN_EMAIL: ${APP_ADMIN_EMAIL:-admin@example.com}
      APP_ADMIN_PASSWORD: ${APP_ADMIN_PASSWORD:?set APP_ADMIN_PASSWORD}
    volumes:
      - media:/data/media

  web:
    build:
      context: web
      dockerfile: Dockerfile
    restart: unless-stopped
    environment:
      PORT: "3000"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    depends_on:
      - api
      - web
    environment:
      APP_DOMAIN: ${APP_DOMAIN}
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - media:/srv/media:ro
      - caddy-data:/data
      - caddy-config:/config

  backup:
    image: postgres:17-alpine
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    entrypoint: ["/bin/sh", "/backup.sh"]
    environment:
      PGHOST: postgres
      PGUSER: ${POSTGRES_USER:-phumpanya}
      PGPASSWORD: ${POSTGRES_PASSWORD}
      PGDATABASE: ${POSTGRES_DB:-phumpanya}
    volumes:
      - ./deploy/backup/backup.sh:/backup.sh:ro
      - pg-backups:/backups

volumes:
  pg-data:
  pg-backups:
  media:
  caddy-data:
  caddy-config:
```

- [ ] **Step 2: Append to `.env.example`**

```
# --- Optional Postgres/Caddy compose stack (docker-compose.stack.yaml) ---
# APP_DOMAIN=example.org
# APP_ADMIN_EMAIL=admin@example.org
# APP_ADMIN_PASSWORD=change-me-long-random
# POSTGRES_DB=phumpanya
# POSTGRES_USER=phumpanya
# POSTGRES_PASSWORD=change-me-long-random
```

- [ ] **Step 3: Verify compose config resolves**

Run: `APP_DOMAIN=example.org APP_ADMIN_PASSWORD=x POSTGRES_PASSWORD=y docker compose -f docker-compose.stack.yaml config >/dev/null && echo CONFIG_OK`
Expected: `CONFIG_OK`.

- [ ] **Step 4: Commit**

```bash
rtk git add docker-compose.stack.yaml .env.example
rtk git commit -m "feat(deploy): compose stack (api, web, postgres, caddy, backup)"
```

---

### Task 12: CI (Postgres integration + stack validate), runbook, CONTEXT

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `docs/ops/deploy-compose.md`
- Modify: `CONTEXT.md`

**Interfaces:**
- Consumes: the gated `TestOpenWithPostgres` (Task 2), the compose file and Caddyfile.
- Produces: a CI job that runs the Go suite against a live Postgres, plus a lint job that validates the compose file and Caddyfile.

- [ ] **Step 1: Add a Postgres integration job to `ci.yml`**

Add this job (adapt `runs-on`/`go-version` to match the existing Go job):

```yaml
  go-postgres:
    name: Go test (postgres)
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_DB: phumpanya
          POSTGRES_USER: phumpanya
          POSTGRES_PASSWORD: ci
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U phumpanya"
          --health-interval 10s --health-timeout 5s --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Test against Postgres
        env:
          APP_DATABASE_URL: postgres://phumpanya:ci@localhost:5432/phumpanya?sslmode=disable
        run: go test ./internal/db/ -run TestOpenWithPostgres -v
```

- [ ] **Step 2: Add a stack-validate job to `ci.yml`**

```yaml
  stack-validate:
    name: Validate compose stack
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: docker compose config
        env:
          APP_DOMAIN: example.org
          APP_ADMIN_PASSWORD: ci
          POSTGRES_PASSWORD: ci
        run: docker compose -f docker-compose.stack.yaml config >/dev/null
      - name: caddy validate
        run: |
          docker run --rm -e APP_DOMAIN=example.org \
            -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" \
            caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
```

- [ ] **Step 3: Verify CI YAML parses**

Run: `rtk python -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo YAML_OK`
Expected: `YAML_OK`. (If Python is unavailable, use any YAML linter.)

- [ ] **Step 4: Write the runbook `docs/ops/deploy-compose.md`**

Write a runbook mirroring `docs/ops/deploy.md`, covering: prerequisites (Docker + Compose, DNS A record, ports 80/443); `.env` setup (`APP_DOMAIN`, `APP_ADMIN_PASSWORD`, `POSTGRES_PASSWORD`); `docker compose -f docker-compose.stack.yaml up -d --build`; verify (`docker compose ps`, `curl https://<domain>/api/health`); the SRS §6.1 UAT (editor→pending→approve→public); backups (pg_dump sidecar, `pg-backups` volume, off-site copy is the owner's job); upgrade (`docker compose pull && up -d`); rollback (previous image tag). State clearly that this is the **optional** path and the single binary (`docs/ops/deploy.md`) stays the default.

- [ ] **Step 5: Update `CONTEXT.md`**

Add a short entry noting the optional Postgres/Caddy compose stack (ADR-0003), the new config vars (`APP_DB_DRIVER`, `APP_DATABASE_URL`, `APP_BEHIND_PROXY`, `APP_PUBLIC_ORIGIN`), and that the default SQLite binary path is unchanged.

- [ ] **Step 6: Commit**

```bash
rtk git add .github/workflows/ci.yml docs/ops/deploy-compose.md CONTEXT.md
rtk git commit -m "ci+docs: postgres integration job, stack validate, compose runbook"
```

---

## Self-Review

**Spec coverage:**
- §3 architecture / routing → Tasks 9, 11. §4 media via Caddy → Task 9 (+ §11 volume). §5 components → Tasks 7–11. §6 code change 1 (driver) → Task 2; change 2 (behind-proxy) → Task 3; change 3 (cookie/origin) → Task 4; change 4 (backup skip) → Task 5. §6 two Next.js build modes → Task 6. §7 tests → each Go task's TDD steps + Task 12 CI. §8 out-of-scope respected (no SSR, no data-migration tool, Postgres stays opt-in).
- Every spec section maps to a task. No gaps.

**Placeholder scan:** No TBD/TODO. Each code step shows real code. Three verification-only steps (Dockerfiles, Caddyfile, compose, script) use concrete validate commands, appropriate for non-Go deliverables. `>`-quoted notes flag facts to confirm against the codebase (model field name, exact login/backup route paths, `public/` dir, `.dockerignore`) — these are guardrails, not deferred work; each has a grep to resolve it in-task.

**Type consistency:** `Dialector`/`OpenWith`/`Open` signatures match between Task 2 definition and its callers. Config methods `UseTLS`/`CookieSecure`/`AllowedOriginHost`/`BackupEnabled` are defined in Task 1 and consumed with identical names in Tasks 2–5. `NEXT_OUTPUT=standalone` is consistent across Tasks 6, 7. Media mount path `/srv/media` matches between Task 9 (Caddyfile root) and Task 11 (volume mount).
