package herb_test

import (
	"encoding/json"
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
	if err := model.AutoMigrate(g); err != nil {
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
	if err := repo.Create(h); err != nil {
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
	r.Use(auth.LoadUser(store, g))
	herb.RegisterRoutes(r, herb.NewRepo(g), &media.Store{Dir: mediaDir})

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
