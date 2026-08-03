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
