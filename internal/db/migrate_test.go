package db

import (
	"path/filepath"
	"testing"
	"time"

	"phum-panya/internal/model"
)

func TestAutoMigrateCreatesLoginAttempts(t *testing.T) {
	g, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	row := model.LoginAttempt{Key: "a@x|1.2.3.4", CreatedAt: time.Now()}
	if err := g.Create(&row).Error; err != nil {
		t.Fatalf("insert LoginAttempt: %v", err)
	}
	var n int64
	if err := g.Model(&model.LoginAttempt{}).Where("key = ?", "a@x|1.2.3.4").Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
