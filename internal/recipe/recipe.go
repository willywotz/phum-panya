// Package recipe provides CRUD access to ตำรับยา (recipe) rows and their
// nested ingredients.
package recipe

import (
	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// Repo provides CRUD access to recipes and ingredients backed by GORM.
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

// NewRepo returns a Repo backed by g, using clk to stamp audit timestamps.
func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// ListByDoctor returns every recipe belonging to doctorID.
func (r *Repo) ListByDoctor(doctorID uint) ([]model.Recipe, error) {
	var recipes []model.Recipe
	err := r.g.Where("doctor_id = ?", doctorID).Find(&recipes).Error
	return recipes, err
}

// GetIngredients returns every ingredient belonging to recipeID.
func (r *Repo) GetIngredients(recipeID uint) ([]model.Ingredient, error) {
	var ings []model.Ingredient
	err := r.g.Where("recipe_id = ?", recipeID).Find(&ings).Error
	return ings, err
}

// Get returns the recipe with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.Recipe, error) {
	var rec model.Recipe
	err := r.g.First(&rec, id).Error
	return rec, err
}

// Create inserts rec and its ings in one transaction, stamping rec's audit
// fields with actorID and the current time.
func (r *Repo) Create(rec *model.Recipe, ings []model.Ingredient, actorID uint) error {
	return db.Tx(r.g, func(tx *gorm.DB) error {
		rec.UpdatedBy = &actorID
		rec.UpdatedAt = r.clk.Now()
		if err := tx.Create(rec).Error; err != nil {
			return err
		}
		return createIngredients(tx, rec.ID, ings)
	})
}

// Update saves all fields of rec and replaces its ingredients with ings, all
// in one transaction, stamping rec's audit fields with actorID and the
// current time. It returns gorm.ErrRecordNotFound if no recipe with rec.ID
// exists.
func (r *Repo) Update(rec *model.Recipe, ings []model.Ingredient, actorID uint) error {
	return db.Tx(r.g, func(tx *gorm.DB) error {
		var existing model.Recipe
		if err := tx.First(&existing, rec.ID).Error; err != nil {
			return err
		}
		rec.UpdatedBy = &actorID
		rec.UpdatedAt = r.clk.Now()
		if err := tx.Save(rec).Error; err != nil {
			return err
		}
		if err := tx.Where("recipe_id = ?", rec.ID).Delete(&model.Ingredient{}).Error; err != nil {
			return err
		}
		return createIngredients(tx, rec.ID, ings)
	})
}

// createIngredients inserts ings for recipeID on tx, resetting any ID so
// each insert produces a new row.
func createIngredients(tx *gorm.DB, recipeID uint, ings []model.Ingredient) error {
	if len(ings) == 0 {
		return nil
	}
	for i := range ings {
		ings[i].ID = 0
		ings[i].RecipeID = recipeID
	}
	return tx.Create(&ings).Error
}

// Delete removes the recipe with id. It returns gorm.ErrRecordNotFound if no
// recipe with id exists. Related ingredients and cases are removed via
// foreign-key cascade.
func (r *Repo) Delete(id uint) error {
	res := r.g.Delete(&model.Recipe{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ResolveDoctor finds the doctor with code, returning its id and whether
// nameForCheck disagrees with the doctor's stored full name (FR-LINK-1). An
// empty nameForCheck never counts as a mismatch. It returns
// gorm.ErrRecordNotFound if no doctor with code exists.
func (r *Repo) ResolveDoctor(code, nameForCheck string) (doctorID uint, mismatch bool, err error) {
	var d model.Doctor
	if err := r.g.Where("code = ?", code).First(&d).Error; err != nil {
		return 0, false, err
	}
	mismatch = nameForCheck != "" && d.FullName != nameForCheck
	return d.ID, mismatch, nil
}
