package auth_test

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// newDB opens a temp DB, migrates it, and seeds one district and one
// central_admin user (id 1). It is reused by later auth tests.
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	district := model.District{Name: "Test", Province: "Test"}
	if err := g.Create(&district).Error; err != nil {
		t.Fatalf("create district: %v", err)
	}
	active := true
	admin := model.User{
		FullName:     "Admin",
		Email:        "admin@x",
		PasswordHash: "hash",
		Role:         "central_admin",
		Active:       &active,
	}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return g
}

func TestSessionCreateLookupDelete(t *testing.T) {
	g := newDB(t)
	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)

	token, err := store.Create(1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	userID, err := store.Lookup(token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if userID != 1 {
		t.Fatalf("userID = %d, want 1", userID)
	}

	if err := store.Delete(token); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Lookup(token); err != auth.ErrNoSession {
		t.Fatalf("Lookup after delete: err = %v, want ErrNoSession", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	g := newDB(t)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	fake := clock.Fake{T: now}
	store := auth.NewSessionStore(g, fake, time.Hour)

	token, err := store.Create(1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	fake.T = now.Add(time.Hour + time.Second)
	store2 := auth.NewSessionStore(g, fake, time.Hour)
	if _, err := store2.Lookup(token); err != auth.ErrNoSession {
		t.Fatalf("Lookup after expiry: err = %v, want ErrNoSession", err)
	}
}
