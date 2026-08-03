// Package caserec provides CRUD access to เคส (case) rows: anonymous
// treatment results linked to one recipe.
package caserec

import (
	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// Repo provides CRUD access to cases backed by GORM.
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

// NewRepo returns a Repo backed by g, using clk to stamp audit timestamps.
func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// ListByRecipe returns every case belonging to recipeID.
func (r *Repo) ListByRecipe(recipeID uint) ([]model.Case, error) {
	var cases []model.Case
	err := r.g.Where("recipe_id = ?", recipeID).Find(&cases).Error
	return cases, err
}

// Get returns the case with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.Case, error) {
	var c model.Case
	err := r.g.First(&c, id).Error
	return c, err
}

// Create inserts c, stamping its audit fields with actorID and the current
// time.
func (r *Repo) Create(c *model.Case, actorID uint) error {
	c.UpdatedBy = &actorID
	c.UpdatedAt = r.clk.Now()
	return r.g.Create(c).Error
}

// Update saves all fields of c, stamping its audit fields with actorID and
// the current time. It returns gorm.ErrRecordNotFound if no case with c.ID
// exists. Existence is checked first because Save, given a primary key with
// no matching row, inserts rather than reports zero rows affected.
func (r *Repo) Update(c *model.Case, actorID uint) error {
	if _, err := r.Get(c.ID); err != nil {
		return err
	}
	c.UpdatedBy = &actorID
	c.UpdatedAt = r.clk.Now()
	return r.g.Save(c).Error
}

// Delete removes the case with id. It returns gorm.ErrRecordNotFound if no
// case with id exists.
func (r *Repo) Delete(id uint) error {
	res := r.g.Delete(&model.Case{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SetPhoto updates the photo path of the case with id. It returns
// gorm.ErrRecordNotFound if no case with id exists.
func (r *Repo) SetPhoto(id uint, path string) error {
	res := r.g.Model(&model.Case{}).Where("id = ?", id).Update("photo", path)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DistrictOf returns the district ID of the doctor who owns the recipe with
// recipeID, resolved via recipe -> doctor. It returns
// gorm.ErrRecordNotFound if no recipe with recipeID exists.
func (r *Repo) DistrictOf(recipeID uint) (uint, error) {
	var rec model.Recipe
	if err := r.g.First(&rec, recipeID).Error; err != nil {
		return 0, err
	}
	var doc model.Doctor
	if err := r.g.First(&doc, rec.DoctorID).Error; err != nil {
		return 0, err
	}
	return doc.DistrictID, nil
}
