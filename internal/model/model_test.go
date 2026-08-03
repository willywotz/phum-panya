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
