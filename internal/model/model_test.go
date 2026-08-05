package model_test

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

func TestAutoMigrateCreatesTables(t *testing.T) {
	g, _ := db.Open(filepath.Join(t.TempDir(), "m.db"))
	if err := model.AutoMigrate(g); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&model.District{}, &model.User{}, &model.Session{}, &model.Doctor{},
		&model.Herb{}, &model.Recipe{}, &model.RecipePhoto{}, &model.Ingredient{}, &model.Case{}} {
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

func TestImportBatchMigratesAndTagsDoctor(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "ib.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dist := model.District{Name: "d", Province: "p"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("district: %v", err)
	}
	b := model.ImportBatch{ImportedBy: 1, SourceFile: "f.xlsx", RowCount: 3, Status: "committed"}
	if err := g.Create(&b).Error; err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: dist.ID, BatchID: &b.ID}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("insert tagged doctor: %v", err)
	}
	var got model.Doctor
	if err := g.First(&got, d.ID).Error; err != nil {
		t.Fatalf("read doctor: %v", err)
	}
	if got.BatchID == nil || *got.BatchID != b.ID {
		t.Fatalf("doctor.BatchID = %v, want %d", got.BatchID, b.ID)
	}
}

func TestHerbProvenanceAndAliasColumns(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	did := uint(3)
	canonical := model.Herb{ThaiName: "ขิง"}
	if err := g.Create(&canonical).Error; err != nil {
		t.Fatalf("canonical: %v", err)
	}
	alias := model.Herb{ThaiName: "ขิงแก่", CreatedByDistrictID: &did, AliasOfID: &canonical.ID}
	if err := g.Create(&alias).Error; err != nil {
		t.Fatalf("insert alias herb: %v", err)
	}
	var got model.Herb
	if err := g.First(&got, alias.ID).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.CreatedByDistrictID == nil || *got.CreatedByDistrictID != did {
		t.Fatalf("CreatedByDistrictID = %v, want %d", got.CreatedByDistrictID, did)
	}
	if got.AliasOfID == nil || *got.AliasOfID != canonical.ID {
		t.Fatalf("AliasOfID = %v, want %d", got.AliasOfID, canonical.ID)
	}
}

// seedRecipeWithLegacyPhoto migrates g, then simulates a database created
// before issue #18: it adds back the recipes.photo column AutoMigrate no
// longer creates and writes value into it directly, bypassing the
// model.Recipe struct (which no longer has a Photo field).
func seedRecipeWithLegacyPhoto(t *testing.T, g *gorm.DB, value string) uint {
	t.Helper()
	if err := g.Exec(`ALTER TABLE recipes ADD COLUMN photo TEXT`).Error; err != nil {
		t.Fatalf("add legacy photo column: %v", err)
	}
	dist := model.District{Name: "d", Province: "p"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("create district: %v", err)
	}
	doc := model.Doctor{Code: "D1", Photo: "-", FullName: "x", DistrictID: dist.ID, Specialty: "y", Status: "active", FirstYear: 2568}
	if err := g.Create(&doc).Error; err != nil {
		t.Fatalf("create doctor: %v", err)
	}
	rec := model.Recipe{Code: "R1", Name: "n", DoctorID: doc.ID, Indication: "i", Preparation: "p", Usage: "u", DataYear: 2568}
	if err := g.Create(&rec).Error; err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	if err := g.Exec(`UPDATE recipes SET photo = ? WHERE id = ?`, value, rec.ID).Error; err != nil {
		t.Fatalf("set legacy photo: %v", err)
	}
	return rec.ID
}

// TestBackfillRecipePhotosMigratesLegacyValueIdempotently proves the
// one-time backfill turns an existing single-column photo value into
// exactly one recipe_photos row, and that running it again never
// duplicates that row.
func TestBackfillRecipePhotosMigratesLegacyValueIdempotently(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "bf.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	recipeID := seedRecipeWithLegacyPhoto(t, g, "uploads/legacy.jpg")

	if err := model.BackfillRecipePhotos(g); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	var photos []model.RecipePhoto
	if err := g.Where("recipe_id = ?", recipeID).Find(&photos).Error; err != nil {
		t.Fatalf("read photos: %v", err)
	}
	if len(photos) != 1 || photos[0].Path != "uploads/legacy.jpg" {
		t.Fatalf("photos = %+v, want one row with path uploads/legacy.jpg", photos)
	}

	if err := model.BackfillRecipePhotos(g); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var n int64
	g.Model(&model.RecipePhoto{}).Where("recipe_id = ?", recipeID).Count(&n)
	if n != 1 {
		t.Fatalf("recipe_photos count after re-run = %d, want 1 (idempotent)", n)
	}
}
