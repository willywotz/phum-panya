// Package caserec provides CRUD access to เคส (case) rows: anonymous
// treatment results linked to one recipe.
package caserec

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
)

// Repo provides CRUD access to cases backed by GORM.
type Repo struct {
	g    *gorm.DB
	clk  clock.Clock
	rev  *revision.Repo
	lock *yearlock.Repo
}

// NewRepo returns a Repo backed by g, using clk to stamp audit timestamps,
// rev to log immediate (admin) writes, and lock to refuse writes into a
// locked data_year.
func NewRepo(g *gorm.DB, clk clock.Clock, rev *revision.Repo, lock *yearlock.Repo) *Repo {
	return &Repo{g: g, clk: clk, rev: rev, lock: lock}
}

// guardYearWrite refuses the write with yearlock.ErrYearLocked if dataYear
// is locked.
func (r *Repo) guardYearWrite(dataYear int) error {
	locked, err := r.lock.IsLocked(dataYear)
	if err != nil {
		return err
	}
	if locked {
		return yearlock.ErrYearLocked
	}
	return nil
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
	if err := r.guardYearWrite(c.DataYear); err != nil {
		return err
	}
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
	if err := r.guardYearWrite(existing.DataYear); err != nil {
		return err
	}
	if c.DataYear != existing.DataYear {
		if err := r.guardYearWrite(c.DataYear); err != nil {
			return err
		}
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
	// editor queued delete: refuse if the row's data_year is locked (admin erasure is exempt)
	var existing model.Case
	if err := r.g.First(&existing, id).Error; err != nil {
		return err
	}
	if err := r.guardYearWrite(existing.DataYear); err != nil {
		return err
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

// SetPhoto updates the case's photo, refusing the write with
// yearlock.ErrYearLocked if its data_year is locked. An admin write
// (immediate) applies the path to the live photo column right away,
// bypassing approval; an editor write stages the path in pending_photo,
// leaving the live photo untouched until a central admin approves it.
// Either way the resulting row is logged as a revision. It returns
// gorm.ErrRecordNotFound if no case with id exists.
func (r *Repo) SetPhoto(id, actorID uint, path string, immediate bool) error {
	existing, err := r.Get(id)
	if err != nil {
		return err
	}
	if err := r.guardYearWrite(existing.DataYear); err != nil {
		return err
	}
	updates := map[string]any{"updated_by": actorID, "updated_at": r.clk.Now()}
	if immediate {
		updates["photo"] = path
	} else {
		updates["pending_photo"] = path
		updates["rejection_reason"] = nil
	}
	if err := r.g.Model(&model.Case{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	c, err := r.Get(id)
	if err != nil {
		return err
	}
	return r.rev.Append("case", id, actorID, model.ActionUpdate, c)
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
