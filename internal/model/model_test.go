package model_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestAutoMigrateCreatesTables(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&model.District{}, &model.User{}, &model.Session{}, &model.Doctor{},
		&model.Herb{}, &model.Recipe{}, &model.Ingredient{}, &model.Case{}} {
		if !g.Migrator().HasTable(m) {
			t.Fatalf("missing table for %T", m)
		}
	}
}

func TestIngredientXORConstraint(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "x.db"))
	_ = model.AutoMigrate(g)
	err := g.Exec(`INSERT INTO ingredients(recipe_id,herb_id,pending_herb_name) VALUES(1,1,'x')`).Error
	if err == nil {
		t.Fatal("expected check constraint to reject row with both herb_id and pending name")
	}
}

// TestUserDeactivatePersists proves a struct-based Updates call that sets
// Active=false is not silently dropped as a Go bool zero-value, and that
// reading the row back reports the login as inactive.
func TestUserDeactivatePersists(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "u.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatal(err)
	}

	active := true
	user := model.User{
		FullName: "Editor", Email: "editor@example.com",
		PasswordHash: "hash", Role: "district_editor", Active: &active,
	}
	if err := g.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	inactive := false
	if err := g.Model(&model.User{}).Where("id = ?", user.ID).
		Updates(model.User{Active: &inactive}).Error; err != nil {
		t.Fatal(err)
	}

	var got model.User
	if err := g.First(&got, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Active == nil || *got.Active {
		t.Fatalf("expected Active=false after deactivate, got %v", got.Active)
	}
}

// TestDeletingDistrictDoesNotWipeDoctor proves Doctor->District has no
// cascade: deleting a District that still owns a Doctor must not remove
// that Doctor.
func TestDeletingDistrictDoesNotWipeDoctor(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "d.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatal(err)
	}

	district := model.District{Name: "Mueang", Province: "Test"}
	if err := g.Create(&district).Error; err != nil {
		t.Fatal(err)
	}
	doctor := model.Doctor{
		Code: "MUE-01", Photo: "p.jpg", FullName: "Doc",
		DistrictID: district.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor).Error; err != nil {
		t.Fatal(err)
	}

	deleteErr := g.Delete(&district).Error
	var got model.Doctor
	readErr := g.First(&got, doctor.ID).Error
	if deleteErr == nil && readErr != nil {
		t.Fatal("district delete succeeded and wiped its doctor via cascade")
	}
}

func TestAutoMigrateCreatesRevisionAndPendingColumns(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rev := model.Revision{EntityType: "doctor", EntityID: 1, ChangedBy: 2, Action: model.ActionCreate, AfterJSON: "{}"}
	if err := g.Create(&rev).Error; err != nil {
		t.Fatalf("insert revision: %v", err)
	}

	district := model.District{Name: "Mueang", Province: "Test"}
	if err := g.Create(&district).Error; err != nil {
		t.Fatalf("insert district: %v", err)
	}
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", DistrictID: district.ID, Specialty: "y", Status: "active", FirstYear: 2568, ReviewState: model.ReviewPending}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("insert doctor: %v", err)
	}
	var got model.Doctor
	if err := g.First(&got, d.ID).Error; err != nil {
		t.Fatalf("read doctor: %v", err)
	}
	if got.ReviewState != model.ReviewPending {
		t.Fatalf("review_state = %q, want %q", got.ReviewState, model.ReviewPending)
	}
}

func TestYearLockMigrates(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1}).Error; err != nil {
		t.Fatalf("insert year lock: %v", err)
	}
	var got model.YearLock
	if err := g.First(&got, "data_year = ?", 2567).Error; err != nil {
		t.Fatalf("read year lock: %v", err)
	}
	if got.DataYear != 2567 || got.LockedBy != 1 {
		t.Fatalf("year lock = %+v, want DataYear 2567 LockedBy 1", got)
	}
}
