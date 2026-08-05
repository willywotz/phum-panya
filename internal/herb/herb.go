// Package herb provides CRUD access to the shared สมุนไพร (herb) catalog and
// reconciliation of ingredients that still reference a pending herb name.
package herb

import (
	"errors"

	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// ErrNotOwner reports that a district editor tried to edit a herb its
// district did not create.
var ErrNotOwner = errors.New("herb: district may edit only herbs it created")

// ErrSelfMerge reports that Merge was called with the same alias and
// canonical id.
var ErrSelfMerge = errors.New("herb: cannot merge a herb into itself")

// ErrChainedMerge reports that Merge's canonical id is itself an alias of
// another herb, which would form an unresolved alias chain.
var ErrChainedMerge = errors.New("herb: canonical herb is itself an alias of another herb")

// Repo provides CRUD access to herbs backed by GORM.
type Repo struct {
	g *gorm.DB
}

// NewRepo returns a Repo backed by g.
func NewRepo(g *gorm.DB) *Repo {
	return &Repo{g: g}
}

// List returns every non-alias herb in the catalog.
func (r *Repo) List() ([]model.Herb, error) {
	var herbs []model.Herb
	err := r.g.Where("alias_of_id IS NULL").Find(&herbs).Error
	return herbs, err
}

// Get returns the herb with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.Herb, error) {
	var h model.Herb
	err := r.g.First(&h, id).Error
	return h, err
}

// Create adds a herb. An editor create is stamped with its district; an admin create
// (createdByDistrictID == nil) has no district provenance.
func (r *Repo) Create(h *model.Herb, createdByDistrictID *uint) error {
	h.CreatedByDistrictID = createdByDistrictID
	return r.g.Create(h).Error
}

// Update edits a herb. A nil editorDistrictID means a central admin (may edit any herb).
// A non-nil value means a district editor, who may edit only a herb its own district
// created. Provenance and alias link are immutable here.
func (r *Repo) Update(h *model.Herb, editorDistrictID *uint) error {
	var existing model.Herb
	if err := r.g.First(&existing, h.ID).Error; err != nil {
		return err
	}
	if editorDistrictID != nil {
		if existing.CreatedByDistrictID == nil || *existing.CreatedByDistrictID != *editorDistrictID {
			return ErrNotOwner
		}
	}
	h.CreatedByDistrictID = existing.CreatedByDistrictID
	h.AliasOfID = existing.AliasOfID
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

// Merge marks alias an alias of canonical and re-points every ingredient from the alias
// to the canonical herb. Returns the number of ingredient rows re-pointed.
//
// Merge rejects a self-merge (aliasID == canonicalID) and a chained merge
// (canonical is itself already an alias), and requires canonical to exist.
// Any herb already an alias of aliasID is re-pointed directly to canonical
// too, so no alias -> alias -> canonical chain forms.
func (r *Repo) Merge(aliasID, canonicalID uint) (int64, error) {
	if aliasID == canonicalID {
		return 0, ErrSelfMerge
	}
	var n int64
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		var canonical model.Herb
		if err := tx.First(&canonical, canonicalID).Error; err != nil {
			return err
		}
		if canonical.AliasOfID != nil {
			return ErrChainedMerge
		}

		if err := tx.Model(&model.Herb{}).Where("alias_of_id = ?", aliasID).
			Update("alias_of_id", canonicalID).Error; err != nil {
			return err
		}

		res := tx.Model(&model.Ingredient{}).Where("herb_id = ?", aliasID).Update("herb_id", canonicalID)
		if res.Error != nil {
			return res.Error
		}
		n = res.RowsAffected
		return tx.Model(&model.Herb{}).Where("id = ?", aliasID).Update("alias_of_id", canonicalID).Error
	})
	return n, err
}

// NearDuplicates returns catalogued (non-alias) herbs whose Thai name contains the query,
// for a save-time near-duplicate warning. Portable LIKE.
func (r *Repo) NearDuplicates(thaiName string) ([]model.Herb, error) {
	var out []model.Herb
	err := r.g.Where("alias_of_id IS NULL AND thai_name LIKE ?", "%"+thaiName+"%").
		Limit(5).Find(&out).Error
	return out, err
}
