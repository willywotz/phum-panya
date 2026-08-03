package recipe_test

import (
	"errors"
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
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/recipe"
)

// recipeAPI wires a recipe router with an admin (id 1) and a district_editor
// (id 2, district 1) session, a doctor in district 1, a doctor in district 2,
// and a seeded herb.
type recipeAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	repo        *recipe.Repo
	adminToken  string
	editorToken string
	doctor1     model.Doctor
	doctor2     model.Doctor
	herb        model.Herb
}

func newRecipeAPI(t *testing.T) *recipeAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "recipe.db"))
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

	doctor1 := model.Doctor{
		Code: "MUE-01", Photo: "p.jpg", FullName: "Somchai",
		DistrictID: d1.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor1).Error; err != nil {
		t.Fatalf("create doctor1: %v", err)
	}
	doctor2 := model.Doctor{
		Code: "MUE-02", Photo: "p.jpg", FullName: "Somsri",
		DistrictID: d2.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor2).Error; err != nil {
		t.Fatalf("create doctor2: %v", err)
	}

	herb := model.Herb{ThaiName: "ขมิ้น"}
	if err := g.Create(&herb).Error; err != nil {
		t.Fatalf("create herb: %v", err)
	}

	active := true
	admin := model.User{
		FullName: "Admin", Email: "admin@x", PasswordHash: "hash",
		Role: "central_admin", Active: &active,
	}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	editor := model.User{
		FullName: "Editor", Email: "editor@x", PasswordHash: "hash",
		Role: "district_editor", DistrictID: &d1.ID, Active: &active,
	}
	if err := g.Create(&editor).Error; err != nil {
		t.Fatalf("create editor: %v", err)
	}

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(admin.ID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	editorToken, err := store.Create(editor.ID)
	if err != nil {
		t.Fatalf("create editor session: %v", err)
	}

	r := gin.New()
	r.Use(auth.LoadUser(store, g))
	repo := recipe.NewRepo(g, clock.Real{})
	recipe.RegisterRoutes(r, repo)

	return &recipeAPI{
		t: t, g: g, r: r, repo: repo,
		adminToken: adminToken, editorToken: editorToken,
		doctor1: doctor1, doctor2: doctor2, herb: herb,
	}
}

func (env *recipeAPI) doAsEditor(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editorToken, body)
}

func (env *recipeAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
}

func (env *recipeAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestCreateRecipeWithMixedIngredientsPersists(t *testing.T) {
	env := newRecipeAPI(t)
	body := `{"code":"REC-01","name":"Yaa Hom","doctor_id":` + strconv.FormatUint(uint64(env.doctor1.ID), 10) + `,
		"indication":"fever","preparation":"boil","usage":"drink","care_stage":"acute","data_year":2565,
		"ingredients":[
			{"herb_id":` + strconv.FormatUint(uint64(env.herb.ID), 10) + `,"amount":"1","unit":"g","note":""},
			{"pending_herb_name":"Unknown Root","amount":"2","unit":"g","note":""}
		]}`
	res := env.doAsEditor("POST", "/api/recipes", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", res.Code, res.Body.String())
	}

	var rec model.Recipe
	if err := env.g.Where("code = ?", "REC-01").First(&rec).Error; err != nil {
		t.Fatalf("reload recipe: %v", err)
	}
	var ings []model.Ingredient
	if err := env.g.Where("recipe_id = ?", rec.ID).Find(&ings).Error; err != nil {
		t.Fatalf("reload ingredients: %v", err)
	}
	if len(ings) != 2 {
		t.Fatalf("ingredient count = %d, want 2", len(ings))
	}
	var haveHerb, havePending bool
	for _, ing := range ings {
		if ing.HerbID != nil {
			haveHerb = true
		}
		if ing.PendingHerbName != nil {
			havePending = true
		}
	}
	if !haveHerb || !havePending {
		t.Fatalf("expected one herb_id row and one pending_herb_name row, got %+v", ings)
	}
}

func TestCreateRecipeIngredientXORViolationRejected(t *testing.T) {
	env := newRecipeAPI(t)
	body := `{"code":"REC-02","name":"Yaa Hom","doctor_id":` + strconv.FormatUint(uint64(env.doctor1.ID), 10) + `,
		"indication":"fever","preparation":"boil","usage":"drink","care_stage":"acute","data_year":2565,
		"ingredients":[
			{"herb_id":` + strconv.FormatUint(uint64(env.herb.ID), 10) + `,"pending_herb_name":"Both","amount":"1","unit":"g","note":""}
		]}`
	res := env.doAsEditor("POST", "/api/recipes", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}

	var count int64
	env.g.Model(&model.Recipe{}).Where("code = ?", "REC-02").Count(&count)
	if count != 0 {
		t.Fatal("recipe should not have been persisted")
	}
}

func TestCreateRecipeCrossDistrictForbidden(t *testing.T) {
	env := newRecipeAPI(t)
	body := `{"code":"REC-03","name":"Yaa Hom","doctor_id":` + strconv.FormatUint(uint64(env.doctor2.ID), 10) + `,
		"indication":"fever","preparation":"boil","usage":"drink","care_stage":"acute","data_year":2565,
		"ingredients":[{"pending_herb_name":"Root","amount":"1","unit":"g","note":""}]}`
	res := env.doAsEditor("POST", "/api/recipes", body)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403, body %s", res.Code, res.Body.String())
	}
}

func TestResolveDoctor(t *testing.T) {
	env := newRecipeAPI(t)

	id, mismatch, err := env.repo.ResolveDoctor("MUE-01", "Wrong Name")
	if err != nil {
		t.Fatalf("ResolveDoctor wrong name: %v", err)
	}
	if id != env.doctor1.ID || !mismatch {
		t.Fatalf("id=%d mismatch=%v, want id=%d mismatch=true", id, mismatch, env.doctor1.ID)
	}

	id, mismatch, err = env.repo.ResolveDoctor("MUE-01", "Somchai")
	if err != nil {
		t.Fatalf("ResolveDoctor matching name: %v", err)
	}
	if id != env.doctor1.ID || mismatch {
		t.Fatalf("id=%d mismatch=%v, want id=%d mismatch=false", id, mismatch, env.doctor1.ID)
	}

	if _, _, err = env.repo.ResolveDoctor("NOPE", "x"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("ResolveDoctor unknown code: err = %v, want ErrRecordNotFound", err)
	}
}
