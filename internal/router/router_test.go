package router_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/media"
	"phum-panya/internal/router"
)

// newDeps builds a minimal router.Deps backed by a temp DB and mediaDir.
// Tests that need a non-default Cfg can override deps.Cfg before calling
// router.NewEngine.
func newDeps(t *testing.T, mediaDir string) router.Deps {
	t.Helper()
	dir := t.TempDir()

	g, err := db.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	clk := clock.Real{}
	return router.Deps{
		Cfg:        config.Config{MediaDir: mediaDir},
		DB:         g,
		Store:      auth.NewSessionStore(g, clk, time.Hour),
		Throttle:   auth.NewThrottle(clk, 100, time.Minute),
		Media:      &media.Store{Dir: mediaDir},
		Clk:        clk,
		BackupDir:  filepath.Join(dir, "backup"),
		BackupKeep: 7,
		DBPath:     filepath.Join(dir, "app.db"),
		MediaDir:   mediaDir,
	}
}

// newEngine builds a minimal, fully wired engine backed by a temp DB and
// mediaDir.
func newEngine(t *testing.T, mediaDir string) http.Handler {
	t.Helper()
	return router.NewEngine(newDeps(t, mediaDir))
}

// TestMediaRouteServesStoredFile confirms a photo saved under MediaDir is
// reachable, without auth, at /media/<relative path>.
func TestMediaRouteServesStoredFile(t *testing.T) {
	mediaDir := filepath.Join(t.TempDir(), "media")
	if err := os.MkdirAll(filepath.Join(mediaDir, "ab"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	want := []byte("fake-jpeg-bytes")
	if err := os.WriteFile(filepath.Join(mediaDir, "ab", "abcd.jpg"), want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	engine := newEngine(t, mediaDir)

	req := httptest.NewRequest(http.MethodGet, "/media/ab/abcd.jpg", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /media/ab/abcd.jpg status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(want) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), want)
	}
}

// TestMediaRouteMissingMediaDirDoesNotServe confirms /media serves nothing
// (not a 500) when MediaDir does not exist yet at startup.
func TestMediaRouteMissingMediaDirDoesNotServe(t *testing.T) {
	mediaDir := filepath.Join(t.TempDir(), "does-not-exist-yet")
	engine := newEngine(t, mediaDir)

	if _, err := os.Stat(mediaDir); err != nil {
		t.Fatalf("MediaDir should be created at startup, stat err: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/media/nope.jpg", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("GET on missing media dir returned 500, want not-500")
	}
}

// TestMediaRouteTraversalBlocked confirms a path-traversal attempt against
// /media never escapes MediaDir.
func TestMediaRouteTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	engine := newEngine(t, mediaDir)

	req := httptest.NewRequest(http.MethodGet, "/media/../secret.txt", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("path traversal served content: %q", rec.Body.String())
	}
}

// TestSameOriginUsesPublicOriginBehindProxy proves the CSRF origin check is
// wired to AllowedOriginHost, not Domain, so it is active behind a proxy.
func TestSameOriginUsesPublicOriginBehindProxy(t *testing.T) {
	mediaDir := t.TempDir()
	deps := newDeps(t, mediaDir)
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

// TestBackupRouteAbsentOnPostgres proves the SQLite-only backup endpoint is not
// mounted when the app runs on Postgres (the pg_dump sidecar owns backups).
func TestBackupRouteAbsentOnPostgres(t *testing.T) {
	mediaDir := t.TempDir()
	deps := newDeps(t, mediaDir)
	deps.Cfg = config.Config{DBDriver: "postgres"}
	engine := router.NewEngine(deps)

	// Same-origin (no-op check, allowedHost empty) POST so only the route
	// registration is under test; expect the route to be absent.
	req := httptest.NewRequest(http.MethodPost, "/api/backup/run", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/backup/run on postgres = %d, want 404", rec.Code)
	}
}
