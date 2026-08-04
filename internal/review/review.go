package review

import (
	"database/sql"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
)

// Repo runs the central-admin approval queue: it promotes or rolls back the on-row
// pending state that district-editor writes leave behind.
type Repo struct {
	g   *gorm.DB
	rev *revision.Repo
}

func NewRepo(g *gorm.DB, rev *revision.Repo) *Repo {
	return &Repo{g: g, rev: rev}
}

// Item is one entry awaiting a decision.
type Item struct {
	EntityType  string `json:"entityType"`
	EntityID    uint   `json:"entityId"`
	Action      string `json:"action"` // create | update | delete
	ReviewState string `json:"reviewState"`
	DistrictID  uint   `json:"districtId"`
}

var (
	errUnknownEntity = errors.New("review: unknown entity type")
	errNotPending    = errors.New("review: nothing pending")
)

// recipePayload mirrors the composite proposal stashed by recipe.Repo.
type recipePayload struct {
	Recipe      model.Recipe       `json:"recipe"`
	Ingredients []model.Ingredient `json:"ingredients"`
}

// revEntry is a revision to log once the promoting transaction commits. The
// revision.Repo writes on its own connection, so appending inside the write
// transaction deadlocks this SQLite driver (SQLITE_BUSY); the codebase logs
// after commit instead (see recipe.Repo).
type revEntry struct {
	entityType string
	entityID   uint
	actorID    uint
	action     string
	after      any
}

func (r *Repo) flush(log []revEntry) error {
	for _, e := range log {
		if err := r.rev.Append(e.entityType, e.entityID, e.actorID, e.action, e.after); err != nil {
			return err
		}
	}
	return nil
}

type queueRow struct {
	EntityID      uint
	DistrictID    uint
	ReviewState   string
	PendingJSON   *string
	PendingDelete bool
}

func itemFrom(entityType string, row queueRow) Item {
	action := model.ActionCreate
	switch {
	case row.PendingDelete:
		action = model.ActionDelete
	case row.PendingJSON != nil:
		action = model.ActionUpdate
	}
	return Item{
		EntityType:  entityType,
		EntityID:    row.EntityID,
		Action:      action,
		ReviewState: row.ReviewState,
		DistrictID:  row.DistrictID,
	}
}

// Queue lists everything awaiting a decision. nil districtID = all districts
// (central admin); a value scopes to one district.
func (r *Repo) Queue(districtID *uint) ([]Item, error) {
	out := []Item{}
	if err := r.queueDoctors(districtID, &out); err != nil {
		return nil, err
	}
	if err := r.queueChild("recipe", "recipes", districtID, &out); err != nil {
		return nil, err
	}
	if err := r.queueChild("case", "cases", districtID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) queueDoctors(districtID *uint, out *[]Item) error {
	q := r.g.Table("doctors").
		Select("id AS entity_id, district_id, review_state, pending_json, pending_delete").
		Where("review_state = ? OR (pending_json IS NOT NULL AND rejection_reason IS NULL) OR (pending_delete = ? AND rejection_reason IS NULL)",
			model.ReviewPending, true)
	if districtID != nil {
		q = q.Where("district_id = ?", *districtID)
	}
	var rows []queueRow
	if err := q.Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		*out = append(*out, itemFrom("doctor", row))
	}
	return nil
}

// queueChild handles recipes and cases, which resolve their district via joins to
// the owning doctor.
func (r *Repo) queueChild(entityType, table string, districtID *uint, out *[]Item) error {
	q := r.g.Table(table).
		Select(table + ".id AS entity_id, doctors.district_id AS district_id, " + table + ".review_state, " + table + ".pending_json, " + table + ".pending_delete")
	if table == "recipes" {
		q = q.Joins("JOIN doctors ON doctors.id = recipes.doctor_id")
	} else {
		q = q.Joins("JOIN recipes ON recipes.id = cases.recipe_id").
			Joins("JOIN doctors ON doctors.id = recipes.doctor_id")
	}
	q = q.Where(table+".review_state = ? OR ("+table+".pending_json IS NOT NULL AND "+table+".rejection_reason IS NULL) OR ("+table+".pending_delete = ? AND "+table+".rejection_reason IS NULL)",
		model.ReviewPending, true)
	if districtID != nil {
		q = q.Where("doctors.district_id = ?", *districtID)
	}
	var rows []queueRow
	if err := q.Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		*out = append(*out, itemFrom(entityType, row))
	}
	return nil
}

// Approve promotes a pending create/edit/delete to the live public state and logs it.
func (r *Repo) Approve(entityType string, entityID, actorID uint) error {
	var log []revEntry
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		switch entityType {
		case "doctor":
			return approveDoctor(tx, entityID, actorID, &log)
		case "recipe":
			return approveRecipe(tx, entityID, actorID, &log)
		case "case":
			return approveCase(tx, entityID, actorID, &log)
		default:
			return errUnknownEntity
		}
	})
	if err != nil {
		return err
	}
	return r.flush(log)
}

func approveDoctor(tx *gorm.DB, id, actorID uint, log *[]revEntry) error {
	var d model.Doctor
	if err := tx.First(&d, id).Error; err != nil {
		return err
	}
	switch {
	case d.PendingDelete:
		if err := tx.Delete(&model.Doctor{}, id).Error; err != nil {
			return err
		}
		*log = append(*log, revEntry{"doctor", id, actorID, model.ActionDelete, d})
		return nil
	case d.PendingJSON != nil:
		var overlay model.Doctor
		if err := json.Unmarshal([]byte(*d.PendingJSON), &overlay); err != nil {
			return err
		}
		overlay.ID = id
		overlay.Photo = d.Photo
		overlay.ReviewState = model.ReviewApproved
		overlay.PendingJSON = nil
		overlay.RejectionReason = nil
		if err := tx.Save(&overlay).Error; err != nil {
			return err
		}
		*log = append(*log, revEntry{"doctor", id, actorID, model.ActionUpdate, overlay})
		return nil
	case d.ReviewState == model.ReviewPending:
		if err := tx.Model(&model.Doctor{}).Where("id = ?", id).
			Updates(map[string]any{"review_state": model.ReviewApproved, "rejection_reason": nil}).Error; err != nil {
			return err
		}
		d.ReviewState = model.ReviewApproved
		*log = append(*log, revEntry{"doctor", id, actorID, model.ActionCreate, d})
		return nil
	default:
		return errNotPending
	}
}

func approveCase(tx *gorm.DB, id, actorID uint, log *[]revEntry) error {
	var c model.Case
	if err := tx.First(&c, id).Error; err != nil {
		return err
	}
	switch {
	case c.PendingDelete:
		if err := tx.Delete(&model.Case{}, id).Error; err != nil {
			return err
		}
		*log = append(*log, revEntry{"case", id, actorID, model.ActionDelete, c})
		return nil
	case c.PendingJSON != nil:
		var overlay model.Case
		if err := json.Unmarshal([]byte(*c.PendingJSON), &overlay); err != nil {
			return err
		}
		if locked, err := yearlock.IsLockedTx(tx, overlay.DataYear); err != nil {
			return err
		} else if locked {
			return yearlock.ErrYearLocked
		}
		overlay.ID = id
		overlay.Photo = c.Photo
		overlay.ReviewState = model.ReviewApproved
		overlay.PendingJSON = nil
		overlay.RejectionReason = nil
		if err := tx.Save(&overlay).Error; err != nil {
			return err
		}
		*log = append(*log, revEntry{"case", id, actorID, model.ActionUpdate, overlay})
		return nil
	case c.ReviewState == model.ReviewPending:
		if locked, err := yearlock.IsLockedTx(tx, c.DataYear); err != nil {
			return err
		} else if locked {
			return yearlock.ErrYearLocked
		}
		if err := tx.Model(&model.Case{}).Where("id = ?", id).
			Updates(map[string]any{"review_state": model.ReviewApproved, "rejection_reason": nil}).Error; err != nil {
			return err
		}
		c.ReviewState = model.ReviewApproved
		*log = append(*log, revEntry{"case", id, actorID, model.ActionCreate, c})
		return nil
	default:
		return errNotPending
	}
}

func approveRecipe(tx *gorm.DB, id, actorID uint, log *[]revEntry) error {
	var rec model.Recipe
	if err := tx.First(&rec, id).Error; err != nil {
		return err
	}
	switch {
	case rec.PendingDelete:
		if err := tx.Delete(&model.Recipe{}, id).Error; err != nil {
			return err
		}
		*log = append(*log, revEntry{"recipe", id, actorID, model.ActionDelete, rec})
		return nil
	case rec.PendingJSON != nil:
		var payload recipePayload
		if err := json.Unmarshal([]byte(*rec.PendingJSON), &payload); err != nil {
			return err
		}
		if locked, err := yearlock.IsLockedTx(tx, payload.Recipe.DataYear); err != nil {
			return err
		} else if locked {
			return yearlock.ErrYearLocked
		}
		newRec := payload.Recipe
		newRec.ID = id
		newRec.Photo = rec.Photo
		newRec.ReviewState = model.ReviewApproved
		newRec.PendingJSON = nil
		newRec.RejectionReason = nil
		if err := tx.Save(&newRec).Error; err != nil {
			return err
		}
		if err := tx.Where("recipe_id = ?", id).Delete(&model.Ingredient{}).Error; err != nil {
			return err
		}
		for i := range payload.Ingredients {
			ing := payload.Ingredients[i]
			ing.ID = 0
			ing.RecipeID = id
			if err := tx.Create(&ing).Error; err != nil {
				return err
			}
		}
		*log = append(*log, revEntry{"recipe", id, actorID, model.ActionUpdate, payload})
		return nil
	case rec.ReviewState == model.ReviewPending:
		if locked, err := yearlock.IsLockedTx(tx, rec.DataYear); err != nil {
			return err
		} else if locked {
			return yearlock.ErrYearLocked
		}
		if err := tx.Model(&model.Recipe{}).Where("id = ?", id).
			Updates(map[string]any{"review_state": model.ReviewApproved, "rejection_reason": nil}).Error; err != nil {
			return err
		}
		rec.ReviewState = model.ReviewApproved
		*log = append(*log, revEntry{"recipe", id, actorID, model.ActionCreate, rec})
		return nil
	default:
		return errNotPending
	}
}

// Reject returns a change to the editor with a reason and logs the reject event.
func (r *Repo) Reject(entityType string, entityID, actorID uint, reason string) error {
	table, ok := entityTable(entityType)
	if !ok {
		return errUnknownEntity
	}
	var updates map[string]any
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		var state string
		var pendingJSON sql.NullString
		var pendingDelete bool
		row := tx.Table(table).Select("review_state, pending_json, pending_delete").Where("id = ?", entityID).Row()
		if err := row.Scan(&state, &pendingJSON, &pendingDelete); err != nil {
			return err
		}
		updates = map[string]any{"rejection_reason": reason}
		switch {
		case state == model.ReviewPending:
			updates["review_state"] = model.ReviewRejected // rejected create
		case pendingDelete:
			updates["pending_delete"] = false // rejected delete clears the flag
		}
		// A rejected edit keeps its pending_json untouched.
		return tx.Table(table).Where("id = ?", entityID).Updates(updates).Error
	})
	if err != nil {
		return err
	}
	return r.rev.Append(entityType, entityID, actorID, model.ActionReject, updates)
}

// ApproveDoctorTree approves the doctor and every pending recipe/case beneath it
// (the admin "approve all pending for this doctor" bulk button). Returns the count.
func (r *Repo) ApproveDoctorTree(doctorID, actorID uint) (int, error) {
	n := 0
	var log []revEntry
	skip := func(err error) bool {
		return errors.Is(err, errNotPending) || errors.Is(err, gorm.ErrRecordNotFound)
	}
	err := db.Tx(r.g, func(tx *gorm.DB) error {
		if err := approveDoctor(tx, doctorID, actorID, &log); err != nil {
			if !skip(err) {
				return err
			}
		} else {
			n++
		}
		var recipeIDs []uint
		if err := tx.Model(&model.Recipe{}).Where("doctor_id = ?", doctorID).Pluck("id", &recipeIDs).Error; err != nil {
			return err
		}
		for _, rid := range recipeIDs {
			if err := approveRecipe(tx, rid, actorID, &log); err != nil {
				if !skip(err) {
					return err
				}
			} else {
				n++
			}
			var caseIDs []uint
			if err := tx.Model(&model.Case{}).Where("recipe_id = ?", rid).Pluck("id", &caseIDs).Error; err != nil {
				return err
			}
			for _, cid := range caseIDs {
				if err := approveCase(tx, cid, actorID, &log); err != nil {
					if !skip(err) {
						return err
					}
				} else {
					n++
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if err := r.flush(log); err != nil {
		return 0, err
	}
	return n, nil
}

func entityTable(entityType string) (string, bool) {
	switch entityType {
	case "doctor":
		return "doctors", true
	case "recipe":
		return "recipes", true
	case "case":
		return "cases", true
	default:
		return "", false
	}
}
