package publicapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"phum-panya/internal/model"
	"phum-panya/internal/publicapi"
)

// seedDistrict creates a district named name.
func (env *publicAPI) seedDistrict(name string) model.District {
	env.t.Helper()
	d := model.District{Name: name, Province: "Test"}
	if err := env.g.Create(&d).Error; err != nil {
		env.t.Fatalf("create district: %v", err)
	}
	return d
}

// seedFullDoctor creates a doctor with explicit name, known-as, district,
// and consent state (no auto-created district, unlike seedDoctor).
func (env *publicAPI) seedFullDoctor(fullName, knownAs string, districtID uint, consented bool) model.Doctor {
	env.t.Helper()
	doc := model.Doctor{
		Code: "DOC-" + fullName, Photo: "p.jpg", FullName: fullName, KnownAs: knownAs,
		DistrictID: districtID, Specialty: "herbal", Status: "active",
		FirstYear: 2020, ConsentObtained: consented,
	}
	if err := env.g.Create(&doc).Error; err != nil {
		env.t.Fatalf("create doctor: %v", err)
	}
	return doc
}

// seedRecipe creates a recipe named name with the given indication under
// doctorID.
func (env *publicAPI) seedRecipe(doctorID uint, name, indication string) model.Recipe {
	env.t.Helper()
	rec := model.Recipe{
		Code: fmt.Sprintf("REC-%d-%s", doctorID, name), Name: name, DoctorID: doctorID,
		Indication: indication, Preparation: "boil", Usage: "drink", DataYear: 2020,
	}
	if err := env.g.Create(&rec).Error; err != nil {
		env.t.Fatalf("create recipe: %v", err)
	}
	return rec
}

// seedHerb creates a herb named thaiName.
func (env *publicAPI) seedHerb(thaiName string) model.Herb {
	env.t.Helper()
	h := model.Herb{ThaiName: thaiName}
	if err := env.g.Create(&h).Error; err != nil {
		env.t.Fatalf("create herb: %v", err)
	}
	return h
}

// seedIngredient links recipeID to herbID.
func (env *publicAPI) seedIngredient(recipeID, herbID uint) {
	env.t.Helper()
	ing := model.Ingredient{RecipeID: recipeID, HerbID: &herbID, Amount: "1", Unit: "g"}
	if err := env.g.Create(&ing).Error; err != nil {
		env.t.Fatalf("create ingredient: %v", err)
	}
}

// seedPendingIngredient links recipeID to a not-yet-catalogued herb name.
func (env *publicAPI) seedPendingIngredient(recipeID uint, pendingHerbName, amount, unit string) {
	env.t.Helper()
	ing := model.Ingredient{RecipeID: recipeID, PendingHerbName: &pendingHerbName, Amount: amount, Unit: unit}
	if err := env.g.Create(&ing).Error; err != nil {
		env.t.Fatalf("create pending ingredient: %v", err)
	}
}

// getDistricts performs a GET and decodes the response as a district list.
func (env *publicAPI) getDistricts(path string) []publicapi.District {
	env.t.Helper()
	rec := env.get(path)
	if rec.Code != http.StatusOK {
		env.t.Fatalf("GET %s: status %d, body %s", path, rec.Code, rec.Body.String())
	}
	var out []publicapi.District
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		env.t.Fatalf("decode districts: %v", err)
	}
	return out
}

// TestPublicDistrictsListsNamesNoAuth confirms GET /api/public/districts
// returns every district's id/name/province without requiring a login, so
// the public filter can show names instead of raw ids.
func TestPublicDistrictsListsNamesNoAuth(t *testing.T) {
	env := newPublicAPI(t)
	d1 := env.seedDistrict("Muang")
	d2 := env.seedDistrict("Sankamphaeng")

	districts := env.getDistricts("/api/public/districts")
	byID := map[uint]publicapi.District{}
	for _, d := range districts {
		byID[d.ID] = d
	}
	if byID[d1.ID].Name != "Muang" {
		t.Errorf("district %d name = %q, want Muang", d1.ID, byID[d1.ID].Name)
	}
	if byID[d2.ID].Name != "Sankamphaeng" {
		t.Errorf("district %d name = %q, want Sankamphaeng", d2.ID, byID[d2.ID].Name)
	}
	if byID[d1.ID].Province != "Test" {
		t.Errorf("district %d province = %q, want Test", d1.ID, byID[d1.ID].Province)
	}
}

// getDoctors performs a GET and decodes the response as a doctor list.
func (env *publicAPI) getDoctors(path string) []publicapi.Doctor {
	env.t.Helper()
	rec := env.get(path)
	if rec.Code != http.StatusOK {
		env.t.Fatalf("GET %s: status %d, body %s", path, rec.Code, rec.Body.String())
	}
	var out []publicapi.Doctor
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		env.t.Fatalf("decode doctors: %v", err)
	}
	return out
}

// getRecipes performs a GET and decodes the response as a recipe list.
func (env *publicAPI) getRecipes(path string) []publicapi.Recipe {
	env.t.Helper()
	rec := env.get(path)
	if rec.Code != http.StatusOK {
		env.t.Fatalf("GET %s: status %d, body %s", path, rec.Code, rec.Body.String())
	}
	var out []publicapi.Recipe
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		env.t.Fatalf("decode recipes: %v", err)
	}
	return out
}

func TestPublicDoctorsKeywordSearch(t *testing.T) {
	env := newPublicAPI(t)
	dist := env.seedDistrict("D1")
	env.seedFullDoctor("Somchai Jaidee", "", dist.ID, true)
	env.seedFullDoctor("Malee Suk", "Somsak", dist.ID, true)
	env.seedFullDoctor("Wichai Meesuk", "", dist.ID, true)

	docs := env.getDoctors("/api/public/doctors?q=som")
	names := map[string]bool{}
	for _, d := range docs {
		names[d.FullName] = true
	}
	if !names["Somchai Jaidee"] {
		t.Error("expected Somchai Jaidee (name match) in results")
	}
	if !names["Malee Suk"] {
		t.Error("expected Malee Suk (known_as match) in results")
	}
	if names["Wichai Meesuk"] {
		t.Error("did not expect Wichai Meesuk in results")
	}
}

func TestPublicDoctorsDistrictFilter(t *testing.T) {
	env := newPublicAPI(t)
	d1 := env.seedDistrict("D1")
	d2 := env.seedDistrict("D2")
	env.seedFullDoctor("Doctor One", "", d1.ID, true)
	env.seedFullDoctor("Doctor Two", "", d2.ID, true)

	docs := env.getDoctors(fmt.Sprintf("/api/public/doctors?district_id=%d", d1.ID))
	if len(docs) != 1 || docs[0].FullName != "Doctor One" {
		t.Fatalf("expected only Doctor One, got %+v", docs)
	}
}

func TestPublicDoctorsBadDistrictID(t *testing.T) {
	env := newPublicAPI(t)
	rec := env.get("/api/public/doctors?district_id=not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicRecipesHerbFilter(t *testing.T) {
	env := newPublicAPI(t)
	dist := env.seedDistrict("D1")
	doc := env.seedFullDoctor("Doc A", "", dist.ID, true)
	turmeric := env.seedHerb("Turmeric")
	ginger := env.seedHerb("Ginger")

	recWithHerb := env.seedRecipe(doc.ID, "Recipe-Turmeric", "cold")
	env.seedIngredient(recWithHerb.ID, turmeric.ID)
	env.seedIngredient(recWithHerb.ID, ginger.ID) // second ingredient, same recipe

	recWithoutHerb := env.seedRecipe(doc.ID, "Recipe-Other", "fever")
	env.seedIngredient(recWithoutHerb.ID, ginger.ID)

	recipes := env.getRecipes(fmt.Sprintf("/api/public/recipes?herb_id=%d", turmeric.ID))
	if len(recipes) != 1 {
		t.Fatalf("expected exactly 1 recipe (no duplicates), got %d: %+v", len(recipes), recipes)
	}
	if recipes[0].Name != "Recipe-Turmeric" {
		t.Fatalf("expected Recipe-Turmeric, got %s", recipes[0].Name)
	}
}

// TestPublicRecipesIncludeIngredients confirms both the recipe list and the
// nested recipes inside a doctor detail expose herb ingredients (data model
// §4.5): the catalog herb's Thai name for a linked HerbID, and the plain
// pending name for a not-yet-catalogued herb.
func TestPublicRecipesIncludeIngredients(t *testing.T) {
	env := newPublicAPI(t)
	dist := env.seedDistrict("D1")
	doc := env.seedFullDoctor("Doc A", "", dist.ID, true)
	ginger := env.seedHerb("ขิง")
	rec := env.seedRecipe(doc.ID, "Recipe-Ing", "cold")
	env.seedIngredient(rec.ID, ginger.ID)
	pendingName := "ฟ้าทะลายโจร"
	env.seedPendingIngredient(rec.ID, pendingName, "2", "ช้อน")

	recipes := env.getRecipes("/api/public/recipes")
	if len(recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d: %+v", len(recipes), recipes)
	}
	names := map[string]bool{}
	for _, ing := range recipes[0].Ingredients {
		names[ing.HerbName] = true
	}
	if !names["ขิง"] {
		t.Errorf("expected catalog herb name ขิง in ingredients, got %+v", recipes[0].Ingredients)
	}
	if !names[pendingName] {
		t.Errorf("expected pending herb name %s in ingredients, got %+v", pendingName, recipes[0].Ingredients)
	}

	detail := env.get(fmt.Sprintf("/api/public/doctors/%d", doc.ID))
	if !strings.Contains(detail.Body.String(), "ขิง") || !strings.Contains(detail.Body.String(), pendingName) {
		t.Fatalf("doctor detail missing ingredient names: %s", detail.Body.String())
	}
}

func TestPublicRecipesBadHerbID(t *testing.T) {
	env := newPublicAPI(t)
	rec := env.get("/api/public/recipes?herb_id=not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicRecipesKeywordAndDistrictSearch(t *testing.T) {
	env := newPublicAPI(t)
	d1 := env.seedDistrict("D1")
	d2 := env.seedDistrict("D2")
	doc1 := env.seedFullDoctor("Doc D1", "", d1.ID, true)
	doc2 := env.seedFullDoctor("Doc D2", "", d2.ID, true)
	env.seedRecipe(doc1.ID, "Cooling Balm", "fever relief")
	env.seedRecipe(doc2.ID, "Warming Rub", "cold relief")

	recipes := env.getRecipes("/api/public/recipes?q=cooling")
	if len(recipes) != 1 || recipes[0].Name != "Cooling Balm" {
		t.Fatalf("expected only Cooling Balm, got %+v", recipes)
	}

	recipes = env.getRecipes(fmt.Sprintf("/api/public/recipes?district_id=%d", d2.ID))
	if len(recipes) != 1 || recipes[0].Name != "Warming Rub" {
		t.Fatalf("expected only Warming Rub, got %+v", recipes)
	}
}

func TestPublicFiltersExcludeUnconsentedDoctor(t *testing.T) {
	env := newPublicAPI(t)
	dist := env.seedDistrict("D1")
	unconsented := env.seedFullDoctor("Secret Healer", "Hidden Name", dist.ID, false)
	herb := env.seedHerb("SecretHerb")
	rec := env.seedRecipe(unconsented.ID, "Secret Recipe", "secret indication")
	env.seedIngredient(rec.ID, herb.ID)

	docs := env.getDoctors("/api/public/doctors?q=secret")
	if len(docs) != 0 {
		t.Fatalf("unconsented doctor leaked via q filter: %+v", docs)
	}
	docs = env.getDoctors(fmt.Sprintf("/api/public/doctors?district_id=%d", dist.ID))
	if len(docs) != 0 {
		t.Fatalf("unconsented doctor leaked via district_id filter: %+v", docs)
	}

	recipes := env.getRecipes("/api/public/recipes?q=secret")
	if len(recipes) != 0 {
		t.Fatalf("unconsented doctor's recipe leaked via q filter: %+v", recipes)
	}
	recipes = env.getRecipes(fmt.Sprintf("/api/public/recipes?district_id=%d", dist.ID))
	if len(recipes) != 0 {
		t.Fatalf("unconsented doctor's recipe leaked via district_id filter: %+v", recipes)
	}
	recipes = env.getRecipes(fmt.Sprintf("/api/public/recipes?herb_id=%d", herb.ID))
	if len(recipes) != 0 {
		t.Fatalf("unconsented doctor's recipe leaked via herb_id filter: %+v", recipes)
	}
}
