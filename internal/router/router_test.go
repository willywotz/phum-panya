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
	"phum-panya/internal/model"
	"phum-panya/internal/router"
)

// newEngine builds a minimal, fully wired engine backed by a temp DB and
// mediaDir.
func newEngine(t *testing.T, mediaDir string) http.Handler {
	t.Helper()
	dir := t.TempDir()

	g, err := db.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	clk := clock.Real{}
	deps := router.Deps{
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
	return router.NewEngine(deps)
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
