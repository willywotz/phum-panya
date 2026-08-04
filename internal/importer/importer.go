package importer

import (
	"fmt"
	"io"

	"gorm.io/gorm"

	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/doctor"
	"phum-panya/internal/herb"
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
		if existingDoctor[d.Code] {
			rep.Skipped = append(rep.Skipped, SkippedRow{SheetDoctors, d.Code, "code already exists"})
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
		if existingRecipe[r.Code] {
			rep.Skipped = append(rep.Skipped, SkippedRow{SheetRecipes, r.Code, "code already exists"})
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
