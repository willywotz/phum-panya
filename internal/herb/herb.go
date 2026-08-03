// Package herb provides CRUD access to the shared สมุนไพร (herb) catalog and
// reconciliation of ingredients that still reference a pending herb name.
package herb

import (
	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// Repo provides CRUD access to herbs backed by GORM.
type Repo struct {
	g *gorm.DB
}

// NewRepo returns a Repo backed by g.
func NewRepo(g *gorm.DB) *Repo {
	return &Repo{g: g}
}

// List returns every herb.
func (r *Repo) List() ([]model.Herb, error) {
	var herbs []model.Herb
	err := r.g.Find(&herbs).Error
	return herbs, err
}

// Get returns the herb with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.Herb, error) {
	var h model.Herb
	err := r.g.First(&h, id).Error
	return h, err
}

// Create inserts h and sets its ID.
func (r *Repo) Create(h *model.Herb) error {
	return r.g.Create(h).Error
}

// Update saves all fields of h. It returns gorm.ErrRecordNotFound if no herb
// with h.ID exists. Existence is checked first because Save, given a
// primary key with no matching row, inserts rather than reports zero rows
// affected.
func (r *Repo) Update(h *model.Herb) error {
	if _, err := r.Get(h.ID); err != nil {
		return err
	}
	return r.g.Save(h).Error
}

// Delete removes the herb with id. It returns gorm.ErrRecordNotFound if no
// herb with id exists.
func (r *Repo) Delete(id uint) error {
	res := r.g.Delete(&model.Herb{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// PendingNames returns every distinct, non-empty pending herb name still
// referenced by an ingredient.
func (r *Repo) PendingNames() ([]string, error) {
	var names []string
	err := r.g.Model(&model.Ingredient{}).
		Distinct("pending_herb_name").
		Where("pending_herb_name IS NOT NULL AND pending_herb_name <> ''").
		Pluck("pending_herb_name", &names).Error
	return names, err
}

// Reconcile links every ingredient whose pending_herb_name equals
// pendingName to herbID, clearing pending_herb_name so the ingredient's
// herb_id/pending_herb_name XOR check constraint stays satisfied. It
// returns the number of ingredients updated.
func (r *Repo) Reconcile(pendingName string, herbID uint) (int64, error) {
	var affected int64
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		res := tx.Model(&model.Ingredient{}).
			Where("pending_herb_name = ?", pendingName).
			Updates(map[string]any{"herb_id": herbID, "pending_herb_name": nil})
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})
	return affected, err
}
