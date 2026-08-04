// Package caserec provides CRUD access to เคส (case) rows: anonymous
// treatment results linked to one recipe.
package caserec

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

// Repo provides CRUD access to cases backed by GORM.
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

// Create inserts a case. Editor creates enter the pending queue; admin
// creates publish immediately and are logged.
func (r *Repo) Create(c *model.Case, actorID uint, immediate bool) error {
	c.UpdatedBy = &actorID
	c.UpdatedAt = r.clk.Now()
	if immediate {
		c.ReviewState = model.ReviewApproved
	} else {
		c.ReviewState = model.ReviewPending
	}
	if err := r.g.Create(c).Error; err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("case", c.ID, actorID, model.ActionCreate, c)
	}
	return nil
}

// Update: admin writes real columns immediately; editor writes stash the
// proposal in pending_json and leave the approved columns visible. It
// returns gorm.ErrRecordNotFound if no case with c.ID exists. Existence is
// checked first because Save, given a primary key with no matching row,
// inserts rather than reports zero rows affected. The existing Photo is
// preserved: photo changes go only through SetPhoto, so an edit here must
// never blank the stored path.
func (r *Repo) Update(c *model.Case, actorID uint, immediate bool) error {
	var existing model.Case
	if err := r.g.First(&existing, c.ID).Error; err != nil {
		return err
	}
	c.Photo = existing.Photo

	if immediate {
		c.UpdatedBy = &actorID
		c.UpdatedAt = r.clk.Now()
		c.ReviewState = model.ReviewApproved
		c.PendingJSON = nil
		c.RejectionReason = nil
		if err := r.g.Save(c).Error; err != nil {
			return err
		}
		return r.rev.Append("case", c.ID, actorID, model.ActionUpdate, c)
	}

	proposal := *c
	proposal.ReviewState = existing.ReviewState
	blob, err := json.Marshal(&proposal)
	if err != nil {
		return err
	}
	overlay := string(blob)
	return r.g.Model(&model.Case{}).Where("id = ?", c.ID).
		Updates(map[string]any{
			"pending_json":     overlay,
			"pending_delete":   false,
			"rejection_reason": nil,
			"updated_by":       actorID,
			"updated_at":       r.clk.Now(),
		}).Error
}

// Delete: admin deletes now (+revision); editor delete is queued as
// pending_delete. It returns gorm.ErrRecordNotFound if no case with id
// exists.
func (r *Repo) Delete(id, actorID uint, immediate bool) error {
	if immediate {
		var existing model.Case
		if err := r.g.First(&existing, id).Error; err != nil {
			return err
		}
		res := r.g.Delete(&model.Case{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.rev.Append("case", id, actorID, model.ActionDelete, existing)
	}
	res := r.g.Model(&model.Case{}).Where("id = ?", id).
		Updates(map[string]any{"pending_delete": true, "rejection_reason": nil})
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
