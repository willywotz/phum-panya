// Package publicapi serves the public, read-only, no-auth API: consented
// doctors, their recipes and cases, and the shared herb catalog. Every
// query selects explicit public columns into a projection struct (never a
// raw model.Doctor/model.Recipe/model.Case), and every doctor/recipe/case
// query filters to doctors.consent_obtained = true, so private fields and
// unconsented healers never leave this package.
package publicapi

import (
	"gorm.io/gorm"

	"phum-panya/internal/model"
)

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
// district attribution and ingredients. It has no audit fields.
type Recipe struct {
	ID           uint               `json:"id"`
	Code         string             `json:"code"`
	Name         string             `json:"name"`
	DoctorID     uint               `json:"doctor_id"`
	Indication   string             `json:"indication"`
	Preparation  string             `json:"preparation"`
	Usage        string             `json:"usage"`
	Caution      string             `json:"caution"`
	CareStage    string             `json:"care_stage"`
	Photos       []string           `json:"photos" gorm:"-"`
	DataYear     int                `json:"data_year"`
	DoctorName   string             `json:"doctor_name"`
	DistrictName string             `json:"district_name"`
	Ingredients  []PublicIngredient `json:"ingredients" gorm:"-"`
}

// PublicIngredient is the public projection of one Ingredient row inside a
// Recipe (data model §4.5): the herb name (the catalog herb's thai_name
// when reconciled, else the still-pending herb name), plus amount/unit/note.
type PublicIngredient struct {
	HerbName string `json:"herb_name"`
	Amount   string `json:"amount"`
	Unit     string `json:"unit"`
	Note     string `json:"note"`
}

var ingredientColumns = []string{
	"COALESCE(herbs.thai_name, ingredients.pending_herb_name) AS herb_name",
	"ingredients.amount", "ingredients.unit", "ingredients.note",
}

var recipeColumns = []string{
	"recipes.id", "recipes.code", "recipes.name", "recipes.doctor_id",
	"recipes.indication", "recipes.preparation", "recipes.usage",
	"recipes.caution", "recipes.care_stage", "recipes.data_year",
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

// District is the public projection of an อำเภอ, used to label the district
// filter with a name instead of a raw id. Every district is public.
type District struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Province string `json:"province"`
}

var districtColumns = []string{"id", "name", "province"}

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
		Where("consent_obtained = ? AND review_state = ?", true, model.ReviewApproved)
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
		Where("id = ? AND consent_obtained = ? AND review_state = ?", id, true, model.ReviewApproved).
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
// attribution and ingredients.
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
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, r.attachRecipeExtras(out)
}

// ListRecipesByDoctor returns every recipe belonging to doctorID, with
// ingredients, provided the doctor has consented. It returns an empty slice
// for an unconsented or missing doctor.
func (r *Repo) ListRecipesByDoctor(doctorID uint) ([]Recipe, error) {
	var out []Recipe
	if err := r.recipeQuery().Where("recipes.doctor_id = ?", doctorID).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, r.attachRecipeExtras(out)
}

// attachRecipeExtras populates the Ingredients and Photos fields of every
// recipe in recipes in place, one query per recipe per field (fine at this
// scale).
func (r *Repo) attachRecipeExtras(recipes []Recipe) error {
	for i := range recipes {
		ings, err := r.ListIngredientsByRecipe(recipes[i].ID)
		if err != nil {
			return err
		}
		recipes[i].Ingredients = ings
		photos, err := r.ListPhotosByRecipe(recipes[i].ID)
		if err != nil {
			return err
		}
		recipes[i].Photos = photos
	}
	return nil
}

// ListPhotosByRecipe returns the stored photo paths of recipeID, in display
// order (data model §4.5: a recipe may have many images).
func (r *Repo) ListPhotosByRecipe(recipeID uint) ([]string, error) {
	out := []string{}
	err := r.g.Table("recipe_photos").
		Where("recipe_id = ?", recipeID).
		Order("sort_order").
		Pluck("path", &out).Error
	return out, err
}

// ListIngredientsByRecipe returns the public ingredients of recipeID: each
// herb's catalog thai_name (when reconciled) or its pending name, plus
// amount/unit/note.
func (r *Repo) ListIngredientsByRecipe(recipeID uint) ([]PublicIngredient, error) {
	out := []PublicIngredient{}
	err := r.g.Table("ingredients").Select(ingredientColumns).
		Joins("LEFT JOIN herbs ON herbs.id = ingredients.herb_id").
		Where("ingredients.recipe_id = ?", recipeID).
		Find(&out).Error
	return out, err
}

// recipeQuery is the shared base query for recipes: it joins doctors and
// districts for attribution and filters to consented doctors only.
func (r *Repo) recipeQuery() *gorm.DB {
	return r.g.Table("recipes").
		Select(recipeColumns).
		Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
		Joins("JOIN districts ON districts.id = doctors.district_id").
		Where("doctors.consent_obtained = ? AND doctors.review_state = ? AND recipes.review_state = ?",
			true, model.ReviewApproved, model.ReviewApproved)
}

// ListCasesByRecipe returns every case belonging to recipeID, provided the
// recipe's doctor has consented.
func (r *Repo) ListCasesByRecipe(recipeID uint) ([]Case, error) {
	var out []Case
	err := r.g.Table("cases").Select(caseColumns).
		Joins("JOIN recipes ON recipes.id = cases.recipe_id").
		Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
		Where("cases.recipe_id = ? AND doctors.consent_obtained = ? AND doctors.review_state = ? AND recipes.review_state = ? AND cases.review_state = ?",
			recipeID, true, model.ReviewApproved, model.ReviewApproved, model.ReviewApproved).
		Find(&out).Error
	return out, err
}

// ListHerbs returns the whole herb catalog, excluding herbs merged into
// another herb as an alias.
func (r *Repo) ListHerbs() ([]Herb, error) {
	var out []Herb
	err := r.g.Table("herbs").Select(herbColumns).Where("alias_of_id IS NULL").Find(&out).Error
	return out, err
}

// ListDistricts returns every district, for labelling the public district
// filter with a name instead of a raw id.
func (r *Repo) ListDistricts() ([]District, error) {
	var out []District
	err := r.g.Table("districts").Select(districtColumns).Find(&out).Error
	return out, err
}
