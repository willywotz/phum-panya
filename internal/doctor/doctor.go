// Package doctor provides CRUD access to หมอพื้นบ้าน (folk doctor) rows.
package doctor

import (
	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// Repo provides CRUD access to doctors backed by GORM.
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

// NewRepo returns a Repo backed by g, using clk to stamp audit timestamps.
func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// ListByDistrict returns every doctor belonging to districtID.
func (r *Repo) ListByDistrict(districtID uint) ([]model.Doctor, error) {
	var doctors []model.Doctor
	err := r.g.Where("district_id = ?", districtID).Find(&doctors).Error
	return doctors, err
}

// Get returns the doctor with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.Doctor, error) {
	var d model.Doctor
	err := r.g.First(&d, id).Error
	return d, err
}

// Create inserts d, stamping its audit fields with actorID and the current
// time.
func (r *Repo) Create(d *model.Doctor, actorID uint) error {
	d.UpdatedBy = &actorID
	d.UpdatedAt = r.clk.Now()
	return r.g.Create(d).Error
}

// Update saves all fields of d, stamping its audit fields with actorID and
// the current time. It returns gorm.ErrRecordNotFound if no doctor with
// d.ID exists. Existence is checked first because Save, given a primary key
// with no matching row, inserts rather than reports zero rows affected.
func (r *Repo) Update(d *model.Doctor, actorID uint) error {
	if _, err := r.Get(d.ID); err != nil {
		return err
	}
	d.UpdatedBy = &actorID
	d.UpdatedAt = r.clk.Now()
	return r.g.Save(d).Error
}

// Delete removes the doctor with id. It returns gorm.ErrRecordNotFound if
// no doctor with id exists. Related recipes and cases are removed via
// foreign-key cascade.
func (r *Repo) Delete(id uint) error {
	res := r.g.Delete(&model.Doctor{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SetPhoto updates the photo path of the doctor with id. It returns
// gorm.ErrRecordNotFound if no doctor with id exists.
func (r *Repo) SetPhoto(id uint, path string) error {
	res := r.g.Model(&model.Doctor{}).Where("id = ?", id).Update("photo", path)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Unpublish clears consent_obtained for the doctor with id, hiding it from
// public view without deleting its rows. It returns gorm.ErrRecordNotFound
// if no doctor with id exists.
func (r *Repo) Unpublish(id uint) error {
	res := r.g.Model(&model.Doctor{}).Where("id = ?", id).Update("consent_obtained", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
