package caserec_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
)

// caseAPI wires a caserec router with an admin (id 1), a district_editor
// (id 2, district 1), a district_editor (id 3, district 2), a doctor in
// each district, and a recipe under each doctor.
type caseAPI struct {
	t            *testing.T
	g            *gorm.DB
	r            *gin.Engine
	repo         *caserec.Repo
	adminToken   string
	editor1Token string
	editor2Token string
	recipe       model.Recipe
	recipe2      model.Recipe
}

func newCaseAPI(t *testing.T) *caseAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "case.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	d1 := model.District{Name: "One", Province: "Test"}
	d2 := model.District{Name: "Two", Province: "Test"}
	if err := g.Create(&d1).Error; err != nil {
		t.Fatalf("create district 1: %v", err)
	}
	if err := g.Create(&d2).Error; err != nil {
		t.Fatalf("create district 2: %v", err)
	}

	doctor := model.Doctor{
		Code: "MUE-01", Photo: "p.jpg", FullName: "Somchai",
		DistrictID: d1.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor).Error; err != nil {
		t.Fatalf("create doctor: %v", err)
	}

	recipe := model.Recipe{
		Code: "REC-01", Name: "Yaa Hom", DoctorID: doctor.ID,
		Indication: "fever", Preparation: "boil", Usage: "drink", DataYear: 2565,
	}
	if err := g.Create(&recipe).Error; err != nil {
		t.Fatalf("create recipe: %v", err)
	}

	doctor2 := model.Doctor{
		Code: "MUE-02", Photo: "p.jpg", FullName: "Somsri",
		DistrictID: d2.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor2).Error; err != nil {
		t.Fatalf("create doctor2: %v", err)
	}
	recipe2 := model.Recipe{
		Code: "REC-02", Name: "Yaa Hom 2", DoctorID: doctor2.ID,
		Indication: "fever", Preparation: "boil", Usage: "drink", DataYear: 2565,
	}
	if err := g.Create(&recipe2).Error; err != nil {
		t.Fatalf("create recipe2: %v", err)
	}

	active := true
	admin := model.User{
		FullName: "Admin", Email: "admin@x", PasswordHash: "hash",
		Role: "central_admin", Active: &active,
	}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	editor1 := model.User{
		FullName: "Editor1", Email: "editor1@x", PasswordHash: "hash",
		Role: "district_editor", DistrictID: &d1.ID, Active: &active,
	}
	if err := g.Create(&editor1).Error; err != nil {
		t.Fatalf("create editor1: %v", err)
	}
	editor2 := model.User{
		FullName: "Editor2", Email: "editor2@x", PasswordHash: "hash",
		Role: "district_editor", DistrictID: &d2.ID, Active: &active,
	}
	if err := g.Create(&editor2).Error; err != nil {
		t.Fatalf("create editor2: %v", err)
	}

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(admin.ID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	editor1Token, err := store.Create(editor1.ID)
	if err != nil {
		t.Fatalf("create editor1 session: %v", err)
	}
	editor2Token, err := store.Create(editor2.ID)
	if err != nil {
		t.Fatalf("create editor2 session: %v", err)
	}

	r := gin.New()
	r.Use(auth.LoadUser(store, g))
	repo := caserec.NewRepo(g, clock.Real{})
	caserec.RegisterRoutes(r, repo, nil)

	return &caseAPI{
		t: t, g: g, r: r, repo: repo,
		adminToken: adminToken, editor1Token: editor1Token, editor2Token: editor2Token,
		recipe: recipe, recipe2: recipe2,
	}
}

// seedCase inserts a case directly on recipeID, bypassing the API.
func (env *caseAPI) seedCase(recipeID uint) model.Case {
	env.t.Helper()
	cs := model.Case{
		RecipeID: recipeID, PatientGender: "female", PatientAgeRange: "30-40",
		Condition: "fever", Treatment: "herbal tea", Result: "cured",
		Duration: "3 days", DataYear: 2565,
	}
	if err := env.repo.Create(&cs, 1); err != nil {
		env.t.Fatalf("seedCase: %v", err)
	}
	return cs
}

func (env *caseAPI) doAsEditor1(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editor1Token, body)
}

func (env *caseAPI) doAsEditor2(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editor2Token, body)
}

func (env *caseAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func (env *caseAPI) caseBody(result string) string {
	return env.caseBodyWithRecipe(env.recipe.ID, result)
}

func (env *caseAPI) caseBodyWithRecipe(recipeID uint, result string) string {
	return `{"recipe_id":` + strconv.FormatUint(uint64(recipeID), 10) + `,
		"patient_gender":"female","patient_age_range":"30-40","condition":"fever",
		"treatment":"herbal tea","result":"` + result + `","duration":"3 days","data_year":2565}`
}

func TestCreateCaseWithValidResultPersists(t *testing.T) {
	env := newCaseAPI(t)
	res := env.doAsEditor1("POST", "/api/cases", env.caseBody("cured"))
	if res.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201, body %s", res.Code, res.Body.String())
	}

	var count int64
	env.g.Model(&model.Case{}).Where("recipe_id = ?", env.recipe.ID).Count(&count)
	if count != 1 {
		t.Fatalf("case count = %d, want 1", count)
	}
}

func TestCreateCaseInvalidResultRejected(t *testing.T) {
	env := newCaseAPI(t)
	res := env.doAsEditor1("POST", "/api/cases", env.caseBody("banana"))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}

	var count int64
	env.g.Model(&model.Case{}).Where("recipe_id = ?", env.recipe.ID).Count(&count)
	if count != 0 {
		t.Fatal("case should not have been persisted")
	}
}

func TestCreateCaseCrossDistrictForbidden(t *testing.T) {
	env := newCaseAPI(t)
	res := env.doAsEditor2("POST", "/api/cases", env.caseBody("cured"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", res.Code, res.Body.String())
	}
}

// TestUpdateCaseCannotReparentToOtherDistrictRecipe proves an editor cannot
// move their own case onto a recipe owned by a district they cannot write:
// the new recipe_id's district must be checked too, not just the case's
// current one.
func TestUpdateCaseCannotReparentToOtherDistrictRecipe(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe2.ID)

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	res := env.doAsEditor2("PUT", path, env.caseBodyWithRecipe(env.recipe.ID, "cured"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", res.Code, res.Body.String())
	}

	var reloaded model.Case
	if err := env.g.First(&reloaded, cs.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if reloaded.RecipeID != env.recipe2.ID {
		t.Fatalf("RecipeID = %d, want unchanged %d", reloaded.RecipeID, env.recipe2.ID)
	}
}

func TestUpdateOtherDistrictCaseForbidden(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	res := env.doAsEditor2("PUT", path, env.caseBodyWithRecipe(env.recipe.ID, "cured"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", res.Code, res.Body.String())
	}
}

func TestDeleteOtherDistrictCaseForbidden(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	res := env.doAsEditor2("DELETE", path, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", res.Code, res.Body.String())
	}

	var count int64
	env.g.Model(&model.Case{}).Where("id = ?", cs.ID).Count(&count)
	if count != 1 {
		t.Fatal("case should not have been deleted")
	}
}

func TestUploadPhotoOnOtherDistrictCaseForbidden(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10) + "/photo"
	req := httptest.NewRequest("POST", path, strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: "session", Value: env.editor2Token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", rec.Code, rec.Body.String())
	}
}
