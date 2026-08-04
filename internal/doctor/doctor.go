// Package doctor provides CRUD access to หมอพื้นบ้าน (folk doctor) rows.
package doctor

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

// Repo provides CRUD access to doctors backed by GORM.
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
	rev *revision.Repo
}

// NewRepo returns a Repo backed by g, using clk to stamp audit timestamps
// and rev to log immediate (admin) writes.
func NewRepo(g *gorm.DB, clk clock.Clock, rev *revision.Repo) *Repo {
	return &Repo{g: g, clk: clk, rev: rev}
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

// Create inserts a doctor. Editor creates enter the pending queue; admin
// creates publish immediately and are logged.
func (r *Repo) Create(d *model.Doctor, actorID uint, immediate bool) error {
	d.UpdatedBy = &actorID
	d.UpdatedAt = r.clk.Now()
	if immediate {
		d.ReviewState = model.ReviewApproved
	} else {
		d.ReviewState = model.ReviewPending
	}
	if err := r.g.Create(d).Error; err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("doctor", d.ID, actorID, model.ActionCreate, d)
	}
	return nil
}

// Update: admin writes real columns immediately; editor writes stash the
// proposal in pending_json and leave the approved columns visible. It
// returns gorm.ErrRecordNotFound if no doctor with d.ID exists. Existence
// is checked first because Save, given a primary key with no matching row,
// inserts rather than reports zero rows affected. The existing Photo is
// preserved: photo changes go only through SetPhoto, so an edit here must
// never blank the stored path.
func (r *Repo) Update(d *model.Doctor, actorID uint, immediate bool) error {
	var existing model.Doctor
	if err := r.g.First(&existing, d.ID).Error; err != nil {
		return err
	}
	d.Photo = existing.Photo

	if immediate {
		d.UpdatedBy = &actorID
		d.UpdatedAt = r.clk.Now()
		d.ReviewState = model.ReviewApproved
		d.PendingJSON = nil
		d.RejectionReason = nil
		if err := r.g.Save(d).Error; err != nil {
			return err
		}
		return r.rev.Append("doctor", d.ID, actorID, model.ActionUpdate, d)
	}

	proposal := *d
	proposal.ReviewState = existing.ReviewState
	blob, err := json.Marshal(&proposal)
	if err != nil {
		return err
	}
	overlay := string(blob)
	return r.g.Model(&model.Doctor{}).Where("id = ?", d.ID).
		Updates(map[string]any{
			"pending_json":     overlay,
			"pending_delete":   false,
			"rejection_reason": nil,
			"updated_by":       actorID,
			"updated_at":       r.clk.Now(),
		}).Error
}

// Delete: admin deletes now (+revision); editor delete is queued as
// pending_delete. It returns gorm.ErrRecordNotFound if no doctor with id
// exists. Related recipes and cases are removed via foreign-key cascade
// when the delete is immediate.
func (r *Repo) Delete(id, actorID uint, immediate bool) error {
	if immediate {
		var existing model.Doctor
		if err := r.g.First(&existing, id).Error; err != nil {
			return err
		}
		res := r.g.Delete(&model.Doctor{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.rev.Append("doctor", id, actorID, model.ActionDelete, existing)
	}
	res := r.g.Model(&model.Doctor{}).Where("id = ?", id).
		Updates(map[string]any{"pending_delete": true, "rejection_reason": nil})
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
