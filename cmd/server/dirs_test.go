package main

import (
	"os"
	"path/filepath"
	"testing"

	"phum-panya/internal/config"
)

func TestEnsureDataDirsCreatesParents(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		DBPath:    filepath.Join(base, "db", "app.db"),
		MediaDir:  filepath.Join(base, "media"),
		BackupDir: filepath.Join(base, "backup"),
	}
	if err := ensureDataDirs(cfg); err != nil {
		t.Fatalf("ensureDataDirs: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(base, "db"),
		filepath.Join(base, "media"),
		filepath.Join(base, "backup"),
	} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s: err=%v", dir, err)
		}
	}
}
