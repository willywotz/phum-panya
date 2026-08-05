package importer

import (
	"fmt"
	"io"

	"gorm.io/gorm"

	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/doctor"
	"phum-panya/internal/herb"
	"phum-panya/internal/model"
	"phum-panya/internal/recipe"
	"phum-panya/internal/yearlock"
)

// Importer loads the canonical template through the domain services and rules.
type Importer struct {
	g     *gorm.DB
	clk   clock.Clock
	doc   *doctor.Repo
	rec   *recipe.Repo
	cas   *caserec.Repo
	herbs *herb.Repo
	lock  *yearlock.Repo
}

func NewImporter(g *gorm.DB, clk clock.Clock, doc *doctor.Repo, rec *recipe.Repo, cas *caserec.Repo, herbs *herb.Repo, lock *yearlock.Repo) *Importer {
	return &Importer{g: g, clk: clk, doc: doc, rec: rec, cas: cas, herbs: herbs, lock: lock}
}

// Report is the dry-run/commit result.
type Report struct {
	DryRun     bool         `json:"dryRun"`
	DoctorsNew int          `json:"doctorsNew"`
	RecipesNew int          `json:"recipesNew"`
	CasesNew   int          `json:"casesNew"`
	Skipped    []SkippedRow `json:"skipped"`
	Errors     []RowError   `json:"errors"`
	BatchID    *uint        `json:"batchId,omitempty"`
}

type SkippedRow struct {
	Sheet  string `json:"sheet"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type RowError struct {
	Sheet   string `json:"sheet"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

// DryRun parses and validates the workbook without writing anything.
func (im *Importer) DryRun(r io.Reader, sourceName string) (*Report, error) {
	parsed, err := ParseWorkbook(r)
	if err != nil {
		return nil, err
	}
	rep := im.validate(parsed)
	rep.DryRun = true
	return rep, nil
}

// Run validates and, if there are no blocking errors, commits the import through the
// domain services in one batch. On any mid-commit error it undoes the batch.
func (im *Importer) Run(r io.Reader, sourceName string, actorID uint) (*Report, error) {
	parsed, err := ParseWorkbook(r)
	if err != nil {
		return nil, err
	}
	rep := im.validate(parsed)
	rep.DryRun = false
	if len(rep.Errors) > 0 {
		return rep, nil // refuse to commit a batch with validation errors
	}

	batch := model.ImportBatch{
		ImportedBy: actorID,
		ImportedAt: im.clk.Now(),
		SourceFile: sourceName,
		Status:     model.ImportBatchStatusCommitted,
	}
	if err := im.g.Create(&batch).Error; err != nil {
		return nil, err
	}
	if err := im.commit(parsed, &batch, actorID); err != nil {
		_ = im.Undo(batch.ID) // compensating rollback
		return nil, err
	}
	rep.BatchID = &batch.ID
	return rep, nil
}

func (im *Importer) commit(p *Parsed, batch *model.ImportBatch, actorID uint) error {
	doctorID := im.codeIDMap("doctors")
	recipeID := im.codeIDMap("recipes")
	count := 0

	for _, d := range p.Doctors {
		if _, exists := doctorID[d.Code]; exists {
			continue // duplicate: skipped (already reported)
		}
		row := model.Doctor{
			Code: d.Code, Photo: "-", FullName: d.FullName, KnownAs: d.KnownAs, Gender: d.Gender,
			BirthYear: d.BirthYear, DistrictID: d.DistrictID, Address: d.Address, Phone: d.Phone,
			Specialty: d.Specialty, YearsExperience: d.YearsExperience, Lineage: d.Lineage,
			Status: d.Status, FirstYear: d.FirstYear, BatchID: &batch.ID,
		}
		if err := im.doc.Create(&row, actorID, true); err != nil {
			return err
		}
		doctorID[d.Code] = row.ID
		count++
	}

	for _, r := range p.Recipes {
		if _, exists := recipeID[r.Code]; exists {
			continue
		}
		did, ok := doctorID[r.DoctorCode]
		if !ok {
			return fmt.Errorf("recipe %s: doctor_code %q unresolved at commit", r.Code, r.DoctorCode)
		}
		row := model.Recipe{
			Code: r.Code, Name: r.Name, DoctorID: did, Indication: r.Indication,
			Preparation: r.Preparation, Usage: r.Usage, Caution: r.Caution,
			CareStage: r.CareStage, DataYear: r.DataYear, BatchID: &batch.ID,
		}
		ings := im.buildIngredients(r.Code, p.Ingredients)
		if err := im.rec.Create(&row, ings, actorID, true); err != nil {
			return err
		}
		recipeID[r.Code] = row.ID
		count++
	}

	for _, c := range p.Cases {
		rid, ok := recipeID[c.RecipeCode]
		if !ok {
			return fmt.Errorf("case: recipe_code %q unresolved at commit", c.RecipeCode)
		}
		row := model.Case{
			RecipeID: rid, PatientGender: c.PatientGender, PatientAgeRange: c.PatientAgeRange,
			Condition: c.Condition, Treatment: c.Treatment, Result: c.Result,
			Duration: c.Duration, DataYear: c.DataYear, BatchID: &batch.ID,
		}
		if err := im.cas.Create(&row, actorID, true); err != nil {
			return err
		}
		count++
	}

	return im.g.Model(batch).Update("row_count", count).Error
}

// buildIngredients resolves each ingredient's herb by Thai name; an unknown herb takes
// the pending-herb path (PendingHerbName), satisfying the Ingredient XOR constraint.
func (im *Importer) buildIngredients(recipeCode string, all []IngredientRow) []model.Ingredient {
	var out []model.Ingredient
	for _, ing := range all {
		if ing.RecipeCode != recipeCode {
			continue
		}
		m := model.Ingredient{Amount: ing.Amount, Unit: ing.Unit, Note: ing.Note}
		var h model.Herb
		if err := im.g.Where("thai_name = ?", ing.HerbName).First(&h).Error; err == nil {
			id := h.ID
			m.HerbID = &id
		} else {
			name := ing.HerbName
			m.PendingHerbName = &name
		}
		out = append(out, m)
	}
	return out
}

func (im *Importer) codeIDMap(table string) map[string]uint {
	type codeRow struct {
		ID   uint
		Code string
	}
	var rows []codeRow
	im.g.Table(table).Select("id, code").Scan(&rows)
	m := make(map[string]uint, len(rows))
	for _, r := range rows {
		m[r.Code] = r.ID
	}
	return m
}

// Undo removes the rows created by a batch and marks it undone. It deletes the batch's own
// cases and recipes first, then deletes a batch doctor ONLY when it has no remaining children,
// so the doctor-delete cascade can never reap another batch's or a manual row's data.
func (im *Importer) Undo(batchID uint) error {
	return db.Tx(im.g, func(tx *gorm.DB) error {
		// Delete this batch's own cases and recipes (FK cascade cleans this batch's ingredients).
		if err := tx.Where("batch_id = ?", batchID).Delete(&model.Case{}).Error; err != nil {
			return err
		}
		if err := tx.Where("batch_id = ?", batchID).Delete(&model.Recipe{}).Error; err != nil {
			return err
		}
		// Delete this batch's doctors ONLY when they have no remaining children, so the
		// doctor-delete cascade cannot reap another batch's recipes/cases.
		var doctorIDs []uint
		if err := tx.Model(&model.Doctor{}).Where("batch_id = ?", batchID).Pluck("id", &doctorIDs).Error; err != nil {
			return err
		}
		for _, id := range doctorIDs {
			var children int64
			if err := tx.Model(&model.Recipe{}).Where("doctor_id = ?", id).Count(&children).Error; err != nil {
				return err
			}
			if children == 0 {
				if err := tx.Delete(&model.Doctor{}, id).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&model.ImportBatch{}).Where("id = ?", batchID).Update("status", model.ImportBatchStatusUndone).Error
	})
}

// validate builds the report: required fields, duplicate-code skips, link resolution,
// district existence, and locked-year refusal. Read-only.
func (im *Importer) validate(p *Parsed) *Report {
	rep := &Report{Skipped: []SkippedRow{}, Errors: []RowError{}}

	existingDoctor := im.codeSet("doctors")
	existingRecipe := im.codeSet("recipes")
	districtExists := im.idSet("districts")

	fileDoctor := map[string]bool{}
	fileRecipe := map[string]bool{}

	for _, d := range p.Doctors {
		if d.Code == "" || d.FullName == "" || d.Specialty == "" || d.Status == "" || d.FirstYear == 0 || d.DistrictID == 0 {
			rep.Errors = append(rep.Errors, RowError{SheetDoctors, d.Code, "missing required field (code, full_name, specialty, status, first_year, district_id)"})
			continue
		}
		if !districtExists[uint64(d.DistrictID)] {
			rep.Errors = append(rep.Errors, RowError{SheetDoctors, d.Code, fmt.Sprintf("district_id %d does not exist", d.DistrictID)})
			continue
		}
		if existingDoctor[d.Code] || fileDoctor[d.Code] {
			rep.Skipped = append(rep.Skipped, SkippedRow{SheetDoctors, d.Code, "duplicate code"})
			continue
		}
		fileDoctor[d.Code] = true
		rep.DoctorsNew++
	}

	for _, r := range p.Recipes {
		if r.Code == "" || r.Name == "" || r.DoctorCode == "" || r.Indication == "" || r.Preparation == "" || r.Usage == "" || r.DataYear == 0 {
			rep.Errors = append(rep.Errors, RowError{SheetRecipes, r.Code, "missing required field"})
			continue
		}
		if !existingDoctor[r.DoctorCode] && !fileDoctor[r.DoctorCode] {
			rep.Errors = append(rep.Errors, RowError{SheetRecipes, r.Code, fmt.Sprintf("doctor_code %q not found", r.DoctorCode)})
			continue
		}
		if locked, err := im.lock.IsLocked(r.DataYear); err == nil && locked {
			rep.Errors = append(rep.Errors, RowError{SheetRecipes, r.Code, fmt.Sprintf("data_year %d is locked", r.DataYear)})
			continue
		}
		if existingRecipe[r.Code] || fileRecipe[r.Code] {
			rep.Skipped = append(rep.Skipped, SkippedRow{SheetRecipes, r.Code, "duplicate code"})
			continue
		}
		fileRecipe[r.Code] = true
		rep.RecipesNew++
	}

	for _, ing := range p.Ingredients {
		if ing.RecipeCode == "" || ing.HerbName == "" {
			rep.Errors = append(rep.Errors, RowError{SheetIngredients, ing.RecipeCode, "missing recipe_code or herb_name"})
			continue
		}
		if !existingRecipe[ing.RecipeCode] && !fileRecipe[ing.RecipeCode] {
			rep.Errors = append(rep.Errors, RowError{SheetIngredients, ing.RecipeCode, fmt.Sprintf("recipe_code %q not found", ing.RecipeCode)})
		}
	}

	for _, c := range p.Cases {
		if c.RecipeCode == "" || c.Condition == "" || c.Result == "" || c.DataYear == 0 {
			rep.Errors = append(rep.Errors, RowError{SheetCases, c.RecipeCode, "missing required field"})
			continue
		}
		if !existingRecipe[c.RecipeCode] && !fileRecipe[c.RecipeCode] {
			rep.Errors = append(rep.Errors, RowError{SheetCases, c.RecipeCode, fmt.Sprintf("recipe_code %q not found", c.RecipeCode)})
			continue
		}
		if locked, err := im.lock.IsLocked(c.DataYear); err == nil && locked {
			rep.Errors = append(rep.Errors, RowError{SheetCases, c.RecipeCode, fmt.Sprintf("data_year %d is locked", c.DataYear)})
			continue
		}
		rep.CasesNew++
	}

	return rep
}

func (im *Importer) codeSet(table string) map[string]bool {
	var codes []string
	im.g.Table(table).Pluck("code", &codes)
	set := make(map[string]bool, len(codes))
	for _, c := range codes {
		set[c] = true
	}
	return set
}

func (im *Importer) idSet(table string) map[uint64]bool {
	var ids []uint64
	im.g.Table(table).Pluck("id", &ids)
	set := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
