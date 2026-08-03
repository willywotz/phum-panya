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
	if err := g.Exec(fmt.Sprintf("VACUUM INTO '%s'", tmp)).Error; err != nil {
		return "", fmt.Errorf("backup: vacuum into: %w", err)
	}
	return tmp, nil
}

// writeZip creates zipPath containing snapshot as app.db and every file
// under mediaDir as media/<relative-path>. A missing mediaDir is skipped.
func writeZip(zipPath, snapshot, mediaDir string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("backup: create zip: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	if err := addFile(zw, snapshot, "app.db"); err != nil {
		return err
	}
	if _, err := os.Stat(mediaDir); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(mediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(mediaDir, path)
		if err != nil {
			return err
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
// order) and removes the rest.
func prune(outDir string, keep int) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("backup: read outDir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
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
