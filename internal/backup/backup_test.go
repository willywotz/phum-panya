package backup_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"phum-panya/internal/backup"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
)

func TestBackupProducesZipAndPrunes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = db.AutoMigrate(g)
	mediaDir := filepath.Join(dir, "media")
	os.MkdirAll(mediaDir, 0o755)
	os.WriteFile(filepath.Join(mediaDir, "x.jpg"), []byte("img"), 0o644)
	outDir := filepath.Join(dir, "out")

	fake := &clock.Fake{T: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	var last string
	for i := 0; i < 5; i++ {
		fake.T = fake.T.AddDate(0, 0, i)
		z, err := backup.Run(dbPath, mediaDir, outDir, 3, fake)
		if err != nil {
			t.Fatal(err)
		}
		last = z
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 3 {
		t.Fatalf("kept %d zips, want 3 (pruned)", len(entries))
	}
	zr, _ := zip.OpenReader(last)
	defer zr.Close()
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	if !got["app.db"] || !got["media/x.jpg"] {
		t.Fatalf("zip missing entries: %v", got)
	}
}

// TestBackupRunRepeatedDoesNotLeakConnections guards against fd/connection
// pool exhaustion: snapshotDB opens a *gorm.DB per Run call, and if the
// underlying *sql.DB isn't closed, the process's open-fd count grows with
// every call. err == nil is not a reliable signal here (db.Open caps its
// pool at 4 conns and this env's ulimit is far above what a few dozen runs
// would leak), so this compares /proc/self/fd counts before and after many
// runs instead.
func TestBackupRunRepeatedDoesNotLeakConnections(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fd-count leak check is linux-only")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = db.AutoMigrate(g)
	outDir := filepath.Join(dir, "out")
	mediaDir := filepath.Join(dir, "media")

	fake := &clock.Fake{T: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	run := func() {
		fake.T = fake.T.AddDate(0, 0, 1)
		if _, err := backup.Run(dbPath, mediaDir, outDir, 100, fake); err != nil {
			t.Fatalf("run: %v", err)
		}
	}

	// Warm up so any one-time fds (lazily opened files, etc.) settle before
	// measuring.
	for i := 0; i < 5; i++ {
		run()
	}

	before := openFDCount(t)
	const iterations = 30
	for i := 0; i < iterations; i++ {
		run()
	}
	after := openFDCount(t)

	const slack = 5
	if after > before+slack {
		t.Fatalf("open fds grew from %d to %d after %d runs (leak)", before, after, iterations)
	}
}

// openFDCount returns the number of open file descriptors for this process.
func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(entries)
}

// TestPruneIgnoresNonBackupFiles ensures a stray file in outDir is left
// alone by pruning and doesn't count toward keep.
func TestPruneIgnoresNonBackupFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = db.AutoMigrate(g)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	strayPath := filepath.Join(outDir, "README.txt")
	os.WriteFile(strayPath, []byte("not a backup"), 0o644)

	fake := &clock.Fake{T: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	for i := 0; i < 3; i++ {
		fake.T = fake.T.AddDate(0, 0, 1)
		if _, err := backup.Run(dbPath, filepath.Join(dir, "media"), outDir, 1, fake); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := os.Stat(strayPath); err != nil {
		t.Fatalf("stray file was removed by prune: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	var backups int
	for _, e := range entries {
		if e.Name() != "README.txt" {
			backups++
		}
	}
	if backups != 1 {
		t.Fatalf("kept %d backup zips, want 1", backups)
	}
}
