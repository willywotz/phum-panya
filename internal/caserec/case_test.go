package caserec_test

import (
	"bytes"
	"errors"
	stdimage "image"
	"image/jpeg"
	"mime/multipart"
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
	"phum-panya/internal/media"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
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
	if err := db.AutoMigrate(g); err != nil {
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
	repo := caserec.NewRepo(g, clock.Real{}, revision.NewRepo(g, clock.Real{}), yearlock.NewRepo(g, clock.Real{}))
	caserec.RegisterRoutes(r, repo, &media.LocalStore{Dir: t.TempDir()})

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
	if err := env.repo.Create(&cs, 1, true); err != nil {
		env.t.Fatalf("seedCase: %v", err)
	}
	return cs
}

func (env *caseAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
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

// TestCreateCaseMissingConditionReturns400 proves caseRequest rejects a
// create body missing the required condition field at bind time.
func TestCreateCaseMissingConditionReturns400(t *testing.T) {
	env := newCaseAPI(t)
	body := `{"recipe_id":` + strconv.FormatUint(uint64(env.recipe.ID), 10) + `,"result":"cured","data_year":2565}`
	res := env.doAsEditor1("POST", "/api/cases", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}
}

// TestCreateCaseInvalidPatientGenderReturns400 proves an out-of-range
// patient_gender is a clean 400 from binding, not a 500 from the
// chk_case_patient_gender DB check.
func TestCreateCaseInvalidPatientGenderReturns400(t *testing.T) {
	env := newCaseAPI(t)
	body := `{"recipe_id":` + strconv.FormatUint(uint64(env.recipe.ID), 10) + `,
		"patient_gender":"bogus","condition":"fever","result":"cured","data_year":2565}`
	res := env.doAsEditor1("POST", "/api/cases", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
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

// TestUpdatePreservesPhoto proves that editing a case after its photo was
// uploaded does not wipe the photo: the frontend PUT body carries no photo
// field, and Update must not blank the stored path.
func TestUpdatePreservesPhoto(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	if err := env.repo.SetPhoto(cs.ID, 1, "uploads/case.jpg", true); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	res := env.doAsEditor1("PUT", path, env.caseBody("better"))
	if res.Code != http.StatusOK {
		t.Fatalf("edit = %d, want 200, body = %s", res.Code, res.Body.String())
	}

	var reloaded model.Case
	if err := env.g.First(&reloaded, cs.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if reloaded.Photo != "uploads/case.jpg" {
		t.Fatalf("Photo = %q, want unchanged %q", reloaded.Photo, "uploads/case.jpg")
	}
}

// TestCaseAdminDeleteIsImmediateAndLogged proves an admin delete removes the
// row right away and appends a delete revision, unlike an editor delete
// which only queues pending_delete.
func TestCaseAdminDeleteIsImmediateAndLogged(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	rec := env.doAsAdmin("DELETE", path, "")
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	var count int64
	env.g.Model(&model.Case{}).Where("id = ?", cs.ID).Count(&count)
	if count != 0 {
		t.Fatalf("admin delete must remove the row")
	}
	env.g.Model(&model.Revision{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?", "case", cs.ID, model.ActionDelete).
		Count(&count)
	if count != 1 {
		t.Fatalf("admin delete must append a delete revision, got %d", count)
	}
}

func TestCaseEditorUpdateRefusedInLockedYear(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)
	env.g.Create(&model.YearLock{DataYear: cs.DataYear, LockedBy: 1})

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10)
	rec := env.doAsEditor1("PUT", path, env.caseBody("better"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("editor update in locked year must be 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestEditorPhotoChangeGoesPending proves an editor photo upload does not
// publish immediately: it stages the path in pending_photo, leaves the live
// photo untouched, and still logs a revision for the proposal.
func TestEditorPhotoChangeGoesPending(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)
	env.g.Model(&model.Case{}).Where("id = ?", cs.ID).Update("photo", "uploads/original.jpg")

	if err := env.repo.SetPhoto(cs.ID, 2, "uploads/proposed.jpg", false); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	var reloaded model.Case
	env.g.First(&reloaded, cs.ID)
	if reloaded.Photo != "uploads/original.jpg" {
		t.Fatalf("Photo = %q, want unchanged %q", reloaded.Photo, "uploads/original.jpg")
	}
	if reloaded.PendingPhoto == nil || *reloaded.PendingPhoto != "uploads/proposed.jpg" {
		t.Fatalf("PendingPhoto = %v, want uploads/proposed.jpg", reloaded.PendingPhoto)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ? AND action = ?", "case", cs.ID, model.ActionUpdate).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("editor photo change update revisions = %d, want 1", revCount)
	}
}

// TestAdminPhotoChangeIsImmediate proves an admin photo upload bypasses
// approval: it writes the live photo column right away and logs a revision.
func TestAdminPhotoChangeIsImmediate(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)

	if err := env.repo.SetPhoto(cs.ID, 1, "uploads/admin.jpg", true); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	var reloaded model.Case
	env.g.First(&reloaded, cs.ID)
	if reloaded.Photo != "uploads/admin.jpg" {
		t.Fatalf("Photo = %q, want uploads/admin.jpg", reloaded.Photo)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ? AND action = ?", "case", cs.ID, model.ActionUpdate).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("admin photo change update revisions = %d, want 1", revCount)
	}
}

// TestPhotoChangeRefusedInLockedYear proves SetPhoto obeys the year lock,
// both directly and through the HTTP photo endpoint.
func TestPhotoChangeRefusedInLockedYear(t *testing.T) {
	env := newCaseAPI(t)
	cs := env.seedCase(env.recipe.ID)
	env.g.Create(&model.YearLock{DataYear: cs.DataYear, LockedBy: 1})

	if err := env.repo.SetPhoto(cs.ID, 1, "uploads/locked.jpg", true); !errors.Is(err, yearlock.ErrYearLocked) {
		t.Fatalf("SetPhoto in locked year = %v, want yearlock.ErrYearLocked", err)
	}

	path := "/api/cases/" + strconv.FormatUint(uint64(cs.ID), 10) + "/photo"
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("photo", "photo.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if err := jpeg.Encode(part, stdimage.NewNRGBA(stdimage.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatalf("encode photo: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: env.editor1Token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("photo upload in locked year = %d, want 409, body=%s", rec.Code, rec.Body.String())
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
