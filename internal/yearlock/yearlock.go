package yearlock

import (
	"errors"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

var (
	ErrYearLocked    = errors.New("yearlock: data_year is locked")
	ErrPendingInYear = errors.New("yearlock: cannot lock a year with pending changes")
)

// Repo locks and unlocks a whole data_year, making its Recipe/Case rows read-only.
type Repo struct {
	g   *gorm.DB
	clk clock.Clock
}

func NewRepo(g *gorm.DB, clk clock.Clock) *Repo {
	return &Repo{g: g, clk: clk}
}

// IsLocked reports whether a data_year is frozen.
func (r *Repo) IsLocked(dataYear int) (bool, error) {
	var n int64
	err := r.g.Model(&model.YearLock{}).Where("data_year = ?", dataYear).Count(&n).Error
	return n > 0, err
}

// Lock freezes a year. It refuses if any Recipe/Case in the year is still pending, so a
// locked year always means "final approved state".
func (r *Repo) Lock(dataYear int, actorID uint) error {
	hasPending := func(table string) (bool, error) {
		var n int64
		err := r.g.Table(table).
			Where("data_year = ? AND (review_state = ? OR (pending_json IS NOT NULL AND rejection_reason IS NULL) OR (pending_delete = ? AND rejection_reason IS NULL))",
				dataYear, model.ReviewPending, true).
			Count(&n).Error
		return n > 0, err
	}
	for _, table := range []string{"recipes", "cases"} {
		pending, err := hasPending(table)
		if err != nil {
			return err
		}
		if pending {
			return ErrPendingInYear
		}
	}
	return r.g.Create(&model.YearLock{DataYear: dataYear, LockedAt: r.clk.Now(), LockedBy: actorID}).Error
}

// Unlock removes a year's freeze.
func (r *Repo) Unlock(dataYear int) error {
	return r.g.Where("data_year = ?", dataYear).Delete(&model.YearLock{}).Error
}

// List returns every locked year, newest first.
func (r *Repo) List() ([]model.YearLock, error) {
	var out []model.YearLock
	err := r.g.Order("data_year DESC").Find(&out).Error
	return out, err
}
