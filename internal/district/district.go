// Package district provides CRUD access to อำเภอ (district) rows.
package district

import (
	"gorm.io/gorm"

	"phum-panya/internal/model"
)

// Repo provides CRUD access to districts backed by GORM.
type Repo struct {
	g *gorm.DB
}

// NewRepo returns a Repo backed by g.
func NewRepo(g *gorm.DB) *Repo {
	return &Repo{g: g}
}

// List returns every district.
func (r *Repo) List() ([]model.District, error) {
	var districts []model.District
	err := r.g.Find(&districts).Error
	return districts, err
}

// Get returns the district with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.District, error) {
	var d model.District
	err := r.g.First(&d, id).Error
	return d, err
}

// Create inserts d and sets its ID.
func (r *Repo) Create(d *model.District) error {
	return r.g.Create(d).Error
}

// Update saves all fields of d. It returns gorm.ErrRecordNotFound if no
// district with d.ID exists. Existence is checked first because Save, given
// a primary key with no matching row, inserts rather than reports zero rows
// affected.
func (r *Repo) Update(d *model.District) error {
	if _, err := r.Get(d.ID); err != nil {
		return err
	}
	return r.g.Save(d).Error
}

// Delete removes the district with id. It returns gorm.ErrRecordNotFound if
// no district with id exists.
func (r *Repo) Delete(id uint) error {
	res := r.g.Delete(&model.District{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
