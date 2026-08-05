package main

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestMigrateDBCreatesSchemaAndSeedsAdmin(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	cfg := config.Config{AdminEmail: "admin@x", AdminPassword: "pw"}
	if err := migrateDB(g, cfg); err != nil {
		t.Fatalf("migrateDB: %v", err)
	}
	var u model.User
	if err := g.Where("email = ?", "admin@x").First(&u).Error; err != nil {
		t.Fatalf("admin not seeded: %v", err)
	}
	if u.Role != model.RoleCentralAdmin {
		t.Fatalf("admin role = %q, want central_admin", u.Role)
	}
}
