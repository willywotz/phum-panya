package db

import (
	"fmt"

	"gorm.io/gorm"

	"phum-panya/internal/model"
)

// AutoMigrate creates or updates the tables for every model.
func AutoMigrate(g *gorm.DB) error {
	if err := g.AutoMigrate(
		&model.District{}, &model.User{}, &model.Session{}, &model.Doctor{},
		&model.Herb{}, &model.Recipe{}, &model.RecipePhoto{}, &model.Ingredient{}, &model.Case{},
		&model.Revision{}, &model.YearLock{}, &model.ImportBatch{}, &model.LoginAttempt{},
	); err != nil {
		return err
	}
	// login_attempts holds ephemeral rate-limit rows; skip the WAL on Postgres.
	// Losing them on a crash is harmless (the throttle just resets).
	if g.Dialector.Name() == "postgres" {
		if err := g.Exec("ALTER TABLE login_attempts SET UNLOGGED").Error; err != nil {
			return fmt.Errorf("set login_attempts unlogged: %w", err)
		}
	}
	return nil
}

// BackfillRecipePhotos migrates each recipe's legacy single-column photo
// into one RecipePhoto row (issue #18: a recipe may hold many photos).
// AutoMigrate never drops columns, so a database created before this change
// still carries the old recipes.photo column; this reads it with a raw
// query before it is abandoned. It is idempotent: a recipe that already has
// any recipe_photos rows is skipped, so calling this on every boot never
// duplicates a photo.
func BackfillRecipePhotos(g *gorm.DB) error {
	if !g.Migrator().HasColumn("recipes", "photo") {
		return nil // fresh database: the legacy column never existed
	}

	var legacy []struct {
		ID    uint
		Photo string
	}
	err := g.Table("recipes").Select("id, photo").
		Where("photo IS NOT NULL AND photo <> ''").
		Where("id NOT IN (?)", g.Table("recipe_photos").Select("DISTINCT recipe_id")).
		Find(&legacy).Error
	if err != nil {
		return err
	}

	for _, rec := range legacy {
		if err := g.Create(&model.RecipePhoto{RecipeID: rec.ID, Path: rec.Photo}).Error; err != nil {
			return err
		}
	}
	return nil
}
