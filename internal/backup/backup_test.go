package backup_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"phum-panya/internal/backup"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestBackupProducesZipAndPrunes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = model.AutoMigrate(g)
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
// pool exhaustion: snapshotDB opens a *gorm.DB per Run call, and if it isn't
// closed, enough repeated Runs eventually fail (hitting the open-file
// limit or connection pool errors).
func TestBackupRunRepeatedDoesNotLeakConnections(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = model.AutoMigrate(g)
	outDir := filepath.Join(dir, "out")

	fake := &clock.Fake{T: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	for i := 0; i < 20; i++ {
		fake.T = fake.T.AddDate(0, 0, 1)
		if _, err := backup.Run(dbPath, filepath.Join(dir, "media"), outDir, 5, fake); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

// TestPruneIgnoresNonBackupFiles ensures a stray file in outDir is left
// alone by pruning and doesn't count toward keep.
func TestPruneIgnoresNonBackupFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, _ := db.Open(dbPath)
	_ = model.AutoMigrate(g)
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
