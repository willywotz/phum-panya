// Package recipe provides CRUD access to ตำรับยา (recipe) rows and their
// nested ingredients.
package recipe

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
)

// recipePayload composes a recipe with its ingredients for the pending-edit
// overlay and the revision log.
type recipePayload struct {
	Recipe      model.Recipe       `json:"recipe"`
	Ingredients []model.Ingredient `json:"ingredients"`
}

// Repo provides CRUD access to recipes and ingredients backed by GORM.
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
// fields with actorID and the current time. Editor creates enter the
// pending queue; admin creates publish immediately and are logged.
func (r *Repo) Create(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error {
	if err := r.guardYearWrite(rec.DataYear); err != nil {
		return err
	}
	if immediate {
		rec.ReviewState = model.ReviewApproved
	} else {
		rec.ReviewState = model.ReviewPending
	}
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		rec.UpdatedBy = &actorID
		rec.UpdatedAt = r.clk.Now()
		if err := tx.Create(rec).Error; err != nil {
			return err
		}
		return createIngredients(tx, rec.ID, ings)
	})
	if err != nil {
		return err
	}
	if immediate {
		return r.rev.Append("recipe", rec.ID, actorID, model.ActionCreate, recipePayload{*rec, ings})
	}
	return nil
}

// Update: admin writes save rec and replace its ingredients with ings
// immediately, all in one transaction, and are logged; editor writes stash
// the recipe+ingredients proposal in pending_json and leave the approved
// columns visible. It returns gorm.ErrRecordNotFound if no recipe with
// rec.ID exists. Existence is checked first because Save, given a primary
// key with no matching row, inserts rather than reports zero rows affected.
// The existing Photo is preserved so an edit here never blanks the stored
// path.
func (r *Repo) Update(rec *model.Recipe, ings []model.Ingredient, actorID uint, immediate bool) error {
	var existing model.Recipe
	if err := r.g.First(&existing, rec.ID).Error; err != nil {
		return err
	}
	if err := r.guardYearWrite(existing.DataYear); err != nil {
		return err
	}
	if rec.DataYear != existing.DataYear {
		if err := r.guardYearWrite(rec.DataYear); err != nil {
			return err
		}
	}
	rec.Photo = existing.Photo

	if immediate {
		rec.ReviewState = model.ReviewApproved
		rec.PendingJSON = nil
		rec.RejectionReason = nil
		err := db.Tx(r.g, func(tx *gorm.DB) error {
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
		if err != nil {
			return err
		}
		return r.rev.Append("recipe", rec.ID, actorID, model.ActionUpdate, recipePayload{*rec, ings})
	}

	proposal := recipePayload{Recipe: *rec, Ingredients: ings}
	proposal.Recipe.ReviewState = existing.ReviewState
	blob, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	overlay := string(blob)
	return r.g.Model(&model.Recipe{}).Where("id = ?", rec.ID).
		Updates(map[string]any{
			"pending_json":     overlay,
			"pending_delete":   false,
			"rejection_reason": nil,
			"updated_by":       actorID,
			"updated_at":       r.clk.Now(),
		}).Error
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

// Delete: admin deletes now (+revision); editor delete is queued as
// pending_delete. It returns gorm.ErrRecordNotFound if no recipe with id
// exists. Related ingredients and cases are removed via foreign-key cascade
// when the delete is immediate.
func (r *Repo) Delete(id, actorID uint, immediate bool) error {
	if immediate {
		var existing model.Recipe
		if err := r.g.First(&existing, id).Error; err != nil {
			return err
		}
		res := r.g.Delete(&model.Recipe{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.rev.Append("recipe", id, actorID, model.ActionDelete, existing)
	}
	// editor queued delete: refuse if the row's data_year is locked (admin erasure is exempt)
	var existing model.Recipe
	if err := r.g.First(&existing, id).Error; err != nil {
		return err
	}
	if err := r.guardYearWrite(existing.DataYear); err != nil {
		return err
	}
	res := r.g.Model(&model.Recipe{}).Where("id = ?", id).
		Updates(map[string]any{"pending_delete": true, "rejection_reason": nil})
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
