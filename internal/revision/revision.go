package revision

import (
	"encoding/json"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// Repo appends and reads the immutable edit history (FR-AUDIT-1, extended).
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// Append writes one immutable revision row. after is marshalled to JSON.
func (r *Repo) Append(entityType string, entityID, changedBy uint, action string, after any) error {
	blob, err := json.Marshal(after)
	if err != nil {
		return err
	}
	rev := model.Revision{
		EntityType: entityType,
		EntityID:   entityID,
		ChangedBy:  changedBy,
		ChangedAt:  r.clk.Now(),
		Action:     action,
		AfterJSON:  string(blob),
	}
	return r.g.Create(&rev).Error
}

// List returns the history for one entity, oldest first.
func (r *Repo) List(entityType string, entityID uint) ([]model.Revision, error) {
	var out []model.Revision
	err := r.g.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("id ASC").Find(&out).Error
	return out, err
}
