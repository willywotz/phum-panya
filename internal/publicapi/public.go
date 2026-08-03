// Package publicapi serves the public, read-only, no-auth API: consented
// doctors, their recipes and cases, and the shared herb catalog. Every
// query selects explicit public columns into a projection struct (never a
// raw model.Doctor/model.Recipe/model.Case), and every doctor/recipe/case
// query filters to doctors.consent_obtained = true, so private fields and
// unconsented healers never leave this package.
package publicapi

import "gorm.io/gorm"

// Doctor is the public projection of a หมอพื้นบ้าน profile. It has no
// phone, consent, or audit fields.
type Doctor struct {
	ID              uint   `json:"id"`
	Code            string `json:"code"`
	Photo           string `json:"photo"`
	FullName        string `json:"full_name"`
	KnownAs         string `json:"known_as"`
	Gender          string `json:"gender"`
	BirthYear       int    `json:"birth_year"`
	DistrictID      uint   `json:"district_id"`
	Address         string `json:"address"`
	Specialty       string `json:"specialty"`
	YearsExperience int    `json:"years_experience"`
	Lineage         string `json:"lineage"`
	Status          string `json:"status"`
	FirstYear       int    `json:"first_year"`
}

// doctorColumns is the explicit public column list for Doctor (data model
// §4). It must never include phone, consent_*, or updated_*.
var doctorColumns = []string{
	"id", "code", "photo", "full_name", "known_as", "gender", "birth_year",
	"district_id", "address", "specialty", "years_experience", "lineage",
	"status", "first_year",
}

// Recipe is the public projection of a ตำรับยา formula, plus doctor/
// district attribution. It has no audit fields.
type Recipe struct {
	ID           uint   `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	DoctorID     uint   `json:"doctor_id"`
	Indication   string `json:"indication"`
	Preparation  string `json:"preparation"`
	Usage        string `json:"usage"`
	Caution      string `json:"caution"`
	CareStage    string `json:"care_stage"`
	Photo        string `json:"photo"`
	DataYear     int    `json:"data_year"`
	DoctorName   string `json:"doctor_name"`
	DistrictName string `json:"district_name"`
}

var recipeColumns = []string{
	"recipes.id", "recipes.code", "recipes.name", "recipes.doctor_id",
	"recipes.indication", "recipes.preparation", "recipes.usage",
	"recipes.caution", "recipes.care_stage", "recipes.photo", "recipes.data_year",
	"doctors.full_name AS doctor_name", "districts.name AS district_name",
}

// Case is the public projection of an already-anonymous เคส treatment
// result. It has no audit fields.
type Case struct {
	ID              uint   `json:"id"`
	RecipeID        uint   `json:"recipe_id"`
	PatientGender   string `json:"patient_gender"`
	PatientAgeRange string `json:"patient_age_range"`
	Condition       string `json:"condition"`
	Treatment       string `json:"treatment"`
	Result          string `json:"result"`
	Duration        string `json:"duration"`
	Photo           string `json:"photo"`
	DataYear        int    `json:"data_year"`
}

var caseColumns = []string{
	"cases.id", "cases.recipe_id", "cases.patient_gender", "cases.patient_age_range",
	"cases.condition", "cases.treatment", "cases.result", "cases.duration",
	"cases.photo", "cases.data_year",
}

// Herb is the public projection of the shared herb catalog. Every herb is
// public; there is no consent concept for herbs.
type Herb struct {
	ID             uint   `json:"id"`
	ThaiName       string `json:"thai_name"`
	LocalName      string `json:"local_name"`
	ScientificName string `json:"scientific_name"`
	Photo          string `json:"photo"`
	PartUsed       string `json:"part_used"`
	Properties     string `json:"properties"`
}

var herbColumns = []string{
	"id", "thai_name", "local_name", "scientific_name", "photo", "part_used", "properties",
}

// Repo provides read-only, consent-filtered access to public data, backed
// by GORM.
type Repo struct {
	g *gorm.DB
}

// NewRepo returns a Repo backed by g.
func NewRepo(g *gorm.DB) *Repo {
	return &Repo{g: g}
}

// DoctorFilter narrows ListDoctors to doctors matching Q (a case-insensitive
// substring of full_name or known_as) and/or DistrictID. A zero value
// matches every consented doctor.
type DoctorFilter struct {
	Q          string
	DistrictID *uint
}

// ListDoctors returns the public fields of every consented doctor matching
// f.
func (r *Repo) ListDoctors(f DoctorFilter) ([]Doctor, error) {
	var out []Doctor
	q := r.g.Table("doctors").Select(doctorColumns).
		Where("consent_obtained = ?", true)
	if f.Q != "" {
		like := "%" + f.Q + "%"
		q = q.Where("(LOWER(full_name) LIKE LOWER(?) OR LOWER(known_as) LIKE LOWER(?))", like, like)
	}
	if f.DistrictID != nil {
		q = q.Where("district_id = ?", *f.DistrictID)
	}
	err := q.Find(&out).Error
	return out, err
}

// GetDoctor returns the consented doctor with id, or gorm.ErrRecordNotFound
// if none exists or the doctor has not consented.
func (r *Repo) GetDoctor(id uint) (Doctor, error) {
	var out Doctor
	err := r.g.Table("doctors").Select(doctorColumns).
		Where("id = ? AND consent_obtained = ?", id, true).
		First(&out).Error
	return out, err
}

// RecipeFilter narrows ListRecipes to recipes matching Q (a case-insensitive
// substring of name or indication), DistrictID (the recipe's doctor's
// district), and/or HerbID (the recipe must have an ingredient with that
// herb). A zero value matches every recipe of a consented doctor.
type RecipeFilter struct {
	Q          string
	DistrictID *uint
	HerbID     *uint
}

// ListRecipes returns every recipe of a consented doctor matching f, with
// attribution.
func (r *Repo) ListRecipes(f RecipeFilter) ([]Recipe, error) {
	var out []Recipe
	q := r.recipeQuery()
	if f.Q != "" {
		like := "%" + f.Q + "%"
		q = q.Where("(LOWER(recipes.name) LIKE LOWER(?) OR LOWER(recipes.indication) LIKE LOWER(?))", like, like)
	}
	if f.DistrictID != nil {
		q = q.Where("doctors.district_id = ?", *f.DistrictID)
	}
	if f.HerbID != nil {
		q = q.Where("recipes.id IN (?)",
			r.g.Table("ingredients").Select("recipe_id").Where("herb_id = ?", *f.HerbID))
	}
	err := q.Find(&out).Error
	return out, err
}

// ListRecipesByDoctor returns every recipe belonging to doctorID, provided
// the doctor has consented. It returns an empty slice for an unconsented or
// missing doctor.
func (r *Repo) ListRecipesByDoctor(doctorID uint) ([]Recipe, error) {
	var out []Recipe
	err := r.recipeQuery().Where("recipes.doctor_id = ?", doctorID).Find(&out).Error
	return out, err
}

// recipeQuery is the shared base query for recipes: it joins doctors and
// districts for attribution and filters to consented doctors only.
func (r *Repo) recipeQuery() *gorm.DB {
	return r.g.Table("recipes").
		Select(recipeColumns).
		Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
		Joins("JOIN districts ON districts.id = doctors.district_id").
		Where("doctors.consent_obtained = ?", true)
}

// ListCasesByRecipe returns every case belonging to recipeID, provided the
// recipe's doctor has consented.
func (r *Repo) ListCasesByRecipe(recipeID uint) ([]Case, error) {
	var out []Case
	err := r.g.Table("cases").Select(caseColumns).
		Joins("JOIN recipes ON recipes.id = cases.recipe_id").
		Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
		Where("cases.recipe_id = ? AND doctors.consent_obtained = ?", recipeID, true).
		Find(&out).Error
	return out, err
}

// ListHerbs returns the whole herb catalog.
func (r *Repo) ListHerbs() ([]Herb, error) {
	var out []Herb
	err := r.g.Table("herbs").Select(herbColumns).Find(&out).Error
	return out, err
}
