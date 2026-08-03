// Package backup snapshots the SQLite database and media directory into a
// single dated zip file, pruning older zips beyond a retention count.
package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
)

// Run snapshots dbPath (via VACUUM INTO) and every file under mediaDir into
// one zip named backup-<date>.zip inside outDir, then prunes outDir to the
// newest keep zips. It returns the path of the zip it just wrote.
func Run(dbPath, mediaDir, outDir string, keep int, clk clock.Clock) (string, error) {
	snapshot, err := snapshotDB(dbPath, clk)
	if err != nil {
		return "", err
	}
	defer os.Remove(snapshot)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("backup: mkdir outDir: %w", err)
	}
	zipPath := filepath.Join(outDir, fmt.Sprintf("backup-%s.zip", clk.Now().Format("2006-01-02")))
	if err := writeZip(zipPath, snapshot, mediaDir); err != nil {
		return "", err
	}
	if err := prune(outDir, keep); err != nil {
		return "", err
	}
	return zipPath, nil
}

// snapshotDB runs VACUUM INTO against dbPath, writing a consistent snapshot
// to a unique temp path derived from clk. The caller must remove the result.
func snapshotDB(dbPath string, clk clock.Clock) (string, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("phum-backup-%d.db", clk.Now().UnixNano()))
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("backup: remove stale snapshot: %w", err)
	}
	g, err := db.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("backup: open db: %w", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return "", fmt.Errorf("backup: get sql.DB: %w", err)
	}
	defer sqlDB.Close()

	if err := g.Exec(fmt.Sprintf("VACUUM INTO '%s'", tmp)).Error; err != nil {
		return "", fmt.Errorf("backup: vacuum into: %w", err)
	}
	return tmp, nil
}

// writeZip creates zipPath containing snapshot as app.db and every file
// under mediaDir as media/<relative-path>. A missing mediaDir is skipped.
// The zip writer's central directory and the file are both closed and
// checked before returning, so a flush failure (e.g. disk full) is
// reported instead of silently producing a truncated zip.
func writeZip(zipPath, snapshot, mediaDir string) (err error) {
	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("backup: create zip: %w", err)
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
	}()

	if err := addFile(zw, snapshot, "app.db"); err != nil {
		return err
	}
	if _, statErr := os.Stat(mediaDir); os.IsNotExist(statErr) {
		return nil
	}
	return filepath.Walk(mediaDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(mediaDir, path)
		if relErr != nil {
			return relErr
		}
		return addFile(zw, path, filepath.Join("media", rel))
	})
}

// addFile copies the contents of srcPath into zw as name.
func addFile(zw *zip.Writer, srcPath, name string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", srcPath, err)
	}
	defer src.Close()

	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("backup: create entry %s: %w", name, err)
	}
	_, err = io.Copy(w, src)
	return err
}

// prune keeps the newest keep backup-*.zip files in outDir (by filename
// order) and removes the rest. Files that don't match the backup-*.zip
// pattern are left alone and don't count toward keep.
func prune(outDir string, keep int) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("backup: read outDir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && isBackupName(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(outDir, names[0])); err != nil {
			return fmt.Errorf("backup: prune %s: %w", names[0], err)
		}
		names = names[1:]
	}
	return nil
}

// isBackupName reports whether name matches the backup-*.zip pattern
// written by Run.
func isBackupName(name string) bool {
	return strings.HasPrefix(name, "backup-") && strings.HasSuffix(name, ".zip")
}
