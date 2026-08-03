package backup_test

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"phum-panya/internal/backup"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// TestBackupRestoreRoundTrip proves a backup zip is actually restorable, not
// just that it exists: it seeds a district/doctor row and a media file,
// backs them up, restores app.db and media/ from the zip into fresh paths,
// then reads the doctor row and the media file back from the restored copies.
func TestBackupRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	g, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatal(err)
	}

	district := model.District{Name: "Restore District", Province: "Test"}
	if err := g.Create(&district).Error; err != nil {
		t.Fatal(err)
	}
	doctor := model.Doctor{
		Code: "RESTORE1", FullName: "Restore Doctor", Photo: "photo.jpg",
		DistrictID: district.ID, Specialty: "herbal", Status: "active", FirstYear: 2560,
	}
	if err := g.Create(&doctor).Error; err != nil {
		t.Fatal(err)
	}

	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaBytes := []byte("original doctor photo bytes")
	if err := os.WriteFile(filepath.Join(mediaDir, "photo.jpg"), mediaBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)}
	zipPath, err := backup.Run(dbPath, mediaDir, outDir, 1, fake)
	if err != nil {
		t.Fatal(err)
	}

	restoredDBPath := filepath.Join(dir, "restored", "app.db")
	restoredMediaDir := filepath.Join(dir, "restored", "media")
	if err := restoreZip(zipPath, restoredDBPath, restoredMediaDir); err != nil {
		t.Fatal(err)
	}

	rg, err := db.Open(restoredDBPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	var got model.Doctor
	if err := rg.Where("code = ?", "RESTORE1").First(&got).Error; err != nil {
		t.Fatalf("read back doctor from restored db: %v", err)
	}
	if got.FullName != doctor.FullName || got.DistrictID != district.ID {
		t.Fatalf("restored doctor = %+v, want full_name=%q district_id=%d", got, doctor.FullName, district.ID)
	}

	gotMedia, err := os.ReadFile(filepath.Join(restoredMediaDir, "photo.jpg"))
	if err != nil {
		t.Fatalf("read restored media file: %v", err)
	}
	if !bytes.Equal(gotMedia, mediaBytes) {
		t.Fatalf("restored media bytes = %q, want %q", gotMedia, mediaBytes)
	}
}

// restoreZip extracts app.db from zipPath to dbPath and every media/ entry
// to mediaDir, mirroring the manual steps in docs/ops/restore.md.
func restoreZip(zipPath, dbPath, mediaDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return err
	}

	for _, f := range zr.File {
		switch {
		case f.Name == "app.db":
			if err := extractZipFile(f, dbPath); err != nil {
				return err
			}
		case strings.HasPrefix(f.Name, "media/"):
			dest := filepath.Join(mediaDir, strings.TrimPrefix(f.Name, "media/"))
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			if err := extractZipFile(f, dest); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractZipFile copies the contents of f into a new file at dest.
func extractZipFile(f *zip.File, dest string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}
