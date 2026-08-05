package herb_test

import (
	"encoding/json"
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
	"phum-panya/internal/herb"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
)

// newDB opens a fresh temp SQLite DB and migrates it.
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "herb.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return g
}

// seedRecipeWithPendingIngredient creates a district, doctor, and recipe,
// then attaches one ingredient with a pending herb name and no HerbID.
func seedRecipeWithPendingIngredient(t *testing.T, g *gorm.DB, pendingName string) model.Ingredient {
	t.Helper()

	d := model.District{Name: "Mueang", Province: "Test"}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("create district: %v", err)
	}

	doctor := model.Doctor{
		Code: "MUE-01", Photo: "p.jpg", FullName: "Doc",
		DistrictID: d.ID, Specialty: "herbal", Status: "active", FirstYear: 2020,
	}
	if err := g.Create(&doctor).Error; err != nil {
		t.Fatalf("create doctor: %v", err)
	}

	recipe := model.Recipe{
		Code: "R-01", Name: "Recipe", DoctorID: doctor.ID,
		Indication: "cold", Preparation: "boil", Usage: "drink", DataYear: 2020,
	}
	if err := g.Create(&recipe).Error; err != nil {
		t.Fatalf("create recipe: %v", err)
	}

	name := pendingName
	ingredient := model.Ingredient{
		RecipeID: recipe.ID, PendingHerbName: &name, Amount: "1", Unit: "g",
	}
	if err := g.Create(&ingredient).Error; err != nil {
		t.Fatalf("create ingredient: %v", err)
	}

	return ingredient
}

func TestPendingNamesAndReconcile(t *testing.T) {
	g := newDB(t)
	repo := herb.NewRepo(g)

	const pendingName = "ฟ้าทะลายโจร"
	ingredient := seedRecipeWithPendingIngredient(t, g, pendingName)

	names, err := repo.PendingNames()
	if err != nil {
		t.Fatalf("PendingNames: %v", err)
	}
	if len(names) != 1 || names[0] != pendingName {
		t.Fatalf("PendingNames = %v, want [%s]", names, pendingName)
	}

	h := &model.Herb{ThaiName: pendingName}
	if err := repo.Create(h, nil); err != nil {
		t.Fatalf("Create herb: %v", err)
	}

	count, err := repo.Reconcile(pendingName, h.ID)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if count != 1 {
		t.Fatalf("Reconcile count = %d, want 1", count)
	}

	var got model.Ingredient
	if err := g.First(&got, ingredient.ID).Error; err != nil {
		t.Fatalf("reload ingredient: %v", err)
	}
	if got.HerbID == nil || *got.HerbID != h.ID {
		t.Fatalf("HerbID = %v, want %d", got.HerbID, h.ID)
	}
	if got.PendingHerbName != nil {
		t.Fatalf("PendingHerbName = %v, want nil", *got.PendingHerbName)
	}

	names, err = repo.PendingNames()
	if err != nil {
		t.Fatalf("PendingNames after reconcile: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("PendingNames after reconcile = %v, want empty", names)
	}
}

// router wires LoadUser and the herb routes with a seeded central_admin
// session and a media.Store rooted at a temp dir.
func router(t *testing.T) (r *gin.Engine, adminToken string, mediaDir string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := newDB(t)

	active := true
	admin := model.User{
		FullName: "Admin", Email: "admin@x", PasswordHash: "hash",
		Role: "central_admin", Active: &active,
	}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}

	mediaDir = t.TempDir()
	r = gin.New()
	r.Use(auth.LoadUser(store, auth.NewGormUsers(g)))
	herb.RegisterRoutes(r, herb.NewRepo(g), &media.LocalStore{Dir: mediaDir})

	return r, adminToken, mediaDir
}

func withCookie(req *http.Request, token string) *http.Request {
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	return req
}

func TestStorageUsageEndpoint(t *testing.T) {
	r, adminToken, _ := router(t)

	req := withCookie(httptest.NewRequest(http.MethodGet, "/api/storage", nil), adminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got struct {
		UsedBytes int64 `json:"used_bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, rec.Body.String())
	}
	if got.UsedBytes != 0 {
		t.Fatalf("used_bytes = %d, want 0 for empty dir", got.UsedBytes)
	}
}

func TestHerbCreateListUpdateDelete(t *testing.T) {
	r, adminToken, _ := router(t)

	createBody := `{"thai_name":"ขมิ้น","local_name":"","scientific_name":"","part_used":"","properties":""}`
	req := withCookie(httptest.NewRequest(http.MethodPost, "/api/herbs", strings.NewReader(createBody)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	var created model.Herb
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}

	req = withCookie(httptest.NewRequest(http.MethodGet, "/api/herbs", nil), adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ขมิ้น") {
		t.Fatalf("list body = %s, want created herb", rec.Body.String())
	}

	updateBody := `{"thai_name":"ขิง","local_name":"","scientific_name":"","part_used":"","properties":""}`
	path := "/api/herbs/" + itoa(created.ID)
	req = withCookie(httptest.NewRequest(http.MethodPut, path, strings.NewReader(updateBody)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	req = withCookie(httptest.NewRequest(http.MethodDelete, path, nil), adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func newRepo(t *testing.T) (*herb.Repo, *gorm.DB) {
	t.Helper()
	g := newDB(t)
	return herb.NewRepo(g), g
}

func TestEditorCreatesAndEditsOwnHerbOnly(t *testing.T) {
	repo, _ := newRepo(t)
	d1, d2 := uint(1), uint(2)

	h := model.Herb{ThaiName: "ฟ้าทะลายโจร"}
	if err := repo.Create(&h, &d1); err != nil {
		t.Fatalf("editor create: %v", err)
	}
	if h.CreatedByDistrictID == nil || *h.CreatedByDistrictID != d1 {
		t.Fatalf("create must stamp provenance, got %v", h.CreatedByDistrictID)
	}
	// another district may not edit it
	h.ThaiName = "แก้ไข"
	if err := repo.Update(&h, &d2); !errors.Is(err, herb.ErrNotOwner) {
		t.Fatalf("want ErrNotOwner, got %v", err)
	}
	// the owning district may
	if err := repo.Update(&h, &d1); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	// admin (nil) may edit any herb
	if err := repo.Update(&h, nil); err != nil {
		t.Fatalf("admin update: %v", err)
	}
}

func TestMergeRepointsIngredients(t *testing.T) {
	repo, g := newRepo(t)
	// FK parents for a recipe + ingredient
	dist := model.District{Name: "d", Province: "p"}
	g.Create(&dist)
	doc := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2560, DistrictID: dist.ID, ReviewState: model.ReviewApproved}
	g.Create(&doc)
	rec := model.Recipe{Code: "R1", Name: "r", DoctorID: doc.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2565, ReviewState: model.ReviewApproved}
	g.Create(&rec)
	canonical := model.Herb{ThaiName: "ขิง"}
	dup := model.Herb{ThaiName: "ขิง (ซ้ำ)"}
	g.Create(&canonical)
	g.Create(&dup)
	g.Create(&model.Ingredient{RecipeID: rec.ID, HerbID: &dup.ID, Amount: "1", Unit: "g"})

	n, err := repo.Merge(dup.ID, canonical.ID)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if n != 1 {
		t.Fatalf("re-pointed = %d, want 1", n)
	}
	var ing model.Ingredient
	g.First(&ing)
	if ing.HerbID == nil || *ing.HerbID != canonical.ID {
		t.Fatalf("ingredient not re-pointed to canonical")
	}
	var alias model.Herb
	g.First(&alias, dup.ID)
	if alias.AliasOfID == nil || *alias.AliasOfID != canonical.ID {
		t.Fatalf("dup must be marked alias of canonical")
	}
}

func TestMergeRejectsSelfMerge(t *testing.T) {
	repo, g := newRepo(t)
	h := model.Herb{ThaiName: "ขิง"}
	g.Create(&h)

	if _, err := repo.Merge(h.ID, h.ID); !errors.Is(err, herb.ErrSelfMerge) {
		t.Fatalf("Merge(self) err = %v, want ErrSelfMerge", err)
	}
}

func TestMergeRejectsChainedMerge(t *testing.T) {
	repo, g := newRepo(t)
	root := model.Herb{ThaiName: "ขิง"}
	g.Create(&root)
	middleAlias := model.Herb{ThaiName: "ขิงแก่", AliasOfID: &root.ID}
	g.Create(&middleAlias)
	newAlias := model.Herb{ThaiName: "ขิงอ่อน"}
	g.Create(&newAlias)

	// middleAlias is itself an alias of root; merging newAlias into it must
	// be rejected so no alias -> alias -> canonical chain forms.
	if _, err := repo.Merge(newAlias.ID, middleAlias.ID); !errors.Is(err, herb.ErrChainedMerge) {
		t.Fatalf("Merge(onto alias) err = %v, want ErrChainedMerge", err)
	}
}

func TestMergeRejectsMissingCanonical(t *testing.T) {
	repo, g := newRepo(t)
	h := model.Herb{ThaiName: "ขิง"}
	g.Create(&h)

	if _, err := repo.Merge(h.ID, 9999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Merge(missing canonical) err = %v, want ErrRecordNotFound", err)
	}
}

func TestMergeRepointsExistingAliasesOfAlias(t *testing.T) {
	repo, g := newRepo(t)
	canonical := model.Herb{ThaiName: "ขิง"}
	g.Create(&canonical)
	alias := model.Herb{ThaiName: "ขิงแก่"}
	g.Create(&alias)
	aliasOfAlias := model.Herb{ThaiName: "ขิงอ่อน", AliasOfID: &alias.ID}
	g.Create(&aliasOfAlias)

	if _, err := repo.Merge(alias.ID, canonical.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	var reloaded model.Herb
	if err := g.First(&reloaded, aliasOfAlias.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AliasOfID == nil || *reloaded.AliasOfID != canonical.ID {
		t.Fatalf("aliasOfAlias.AliasOfID = %v, want %d (re-pointed, not chained)", reloaded.AliasOfID, canonical.ID)
	}
}

func TestListExcludesMergedAliases(t *testing.T) {
	repo, g := newRepo(t)
	canonical := model.Herb{ThaiName: "ขิง"}
	g.Create(&canonical)
	alias := model.Herb{ThaiName: "ขิงแก่"}
	g.Create(&alias)

	if _, err := repo.Merge(alias.ID, canonical.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	herbs, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, h := range herbs {
		if h.ID == alias.ID {
			t.Fatalf("List() returned merged alias %d", alias.ID)
		}
	}
	if len(herbs) != 1 || herbs[0].ID != canonical.ID {
		t.Fatalf("List() = %+v, want only canonical", herbs)
	}
}

func TestNearDuplicatesWarns(t *testing.T) {
	repo, g := newRepo(t)
	g.Create(&model.Herb{ThaiName: "ขิง"})
	got, err := repo.NearDuplicates("ขิ")
	if err != nil {
		t.Fatalf("near dup: %v", err)
	}
	if len(got) != 1 || got[0].ThaiName != "ขิง" {
		t.Fatalf("near duplicates = %+v, want ขิง", got)
	}
}
