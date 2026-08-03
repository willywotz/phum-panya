package bootstrap_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/auth"
	"phum-panya/internal/bootstrap"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestEnsureAdminIsIdempotent(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "b.db"))
	_ = model.AutoMigrate(g)

	created, err := bootstrap.EnsureAdmin(g, "admin@x", "pw123456")
	if err != nil || !created {
		t.Fatalf("first ensure: created=%v err=%v", created, err)
	}
	created2, _ := bootstrap.EnsureAdmin(g, "admin@x", "pw123456")
	if created2 {
		t.Fatal("second ensure should not create a duplicate admin")
	}
	var n int64
	g.Model(&model.User{}).Where("role = ?", "central_admin").Count(&n)
	if n != 1 {
		t.Fatalf("admin count = %d, want 1", n)
	}
	var u model.User
	g.Where("email = ?", "admin@x").First(&u)
	if !auth.CheckPassword(u.PasswordHash, "pw123456") {
		t.Fatal("bootstrapped password does not verify")
	}
}
