package publicapi_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/publicapi"
)

// publicAPI wires the public router onto a fresh SQLite DB.
type publicAPI struct {
	t *testing.T
	g *gorm.DB
	r *gin.Engine
}

func newPublicAPI(t *testing.T) *publicAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "public.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	r := gin.New()
	publicapi.RegisterRoutes(r, publicapi.NewRepo(g))

	return &publicAPI{t: t, g: g, r: r}
}

// seedDoctor creates a district, a doctor named name with the given consent
// state (and a phone number, to prove it never leaks publicly), and a
// recipe named recipeName under that doctor.
func (env *publicAPI) seedDoctor(name string, consented bool, recipeName string) {
	env.t.Helper()

	d := model.District{Name: "District-" + name, Province: "Test"}
	if err := env.g.Create(&d).Error; err != nil {
		env.t.Fatalf("create district: %v", err)
	}

	doc := model.Doctor{
		Code: "DOC-" + name, Photo: "p.jpg", FullName: name,
		DistrictID: d.ID, Phone: "0812345678", Specialty: "herbal",
		Status: "active", FirstYear: 2020, ConsentObtained: consented,
	}
	if err := env.g.Create(&doc).Error; err != nil {
		env.t.Fatalf("create doctor: %v", err)
	}

	rec := model.Recipe{
		Code: "REC-" + name, Name: recipeName, DoctorID: doc.ID,
		Indication: "cold", Preparation: "boil", Usage: "drink", DataYear: 2020,
	}
	if err := env.g.Create(&rec).Error; err != nil {
		env.t.Fatalf("create recipe: %v", err)
	}
}

// seedDoctorState creates a district, a doctor named name with the given
// consent and review state, and a recipe named recipeName under that
// doctor with the same review state.
func (env *publicAPI) seedDoctorState(t *testing.T, name string, consented bool, reviewState, recipeName string) {
	t.Helper()

	d := model.District{Name: "District-" + name, Province: "Test"}
	if err := env.g.Create(&d).Error; err != nil {
		t.Fatalf("create district: %v", err)
	}

	doc := model.Doctor{
		Code: "DOC-" + name, Photo: "p.jpg", FullName: name,
		DistrictID: d.ID, Phone: "0812345678", Specialty: "herbal",
		Status: "active", FirstYear: 2020, ConsentObtained: consented,
		ReviewState: reviewState,
	}
	if err := env.g.Create(&doc).Error; err != nil {
		t.Fatalf("create doctor: %v", err)
	}

	rec := model.Recipe{
		Code: "REC-" + name, Name: recipeName, DoctorID: doc.ID,
		Indication: "cold", Preparation: "boil", Usage: "drink", DataYear: 2020,
		ReviewState: reviewState,
	}
	if err := env.g.Create(&rec).Error; err != nil {
		t.Fatalf("create recipe: %v", err)
	}
}

// get performs an unauthenticated GET against the public router.
func (env *publicAPI) get(path string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestPublicHidesUnconsentedAndPrivateFields(t *testing.T) {
	env := newPublicAPI(t)
	env.seedDoctor("A", true, "recipe-A")
	env.seedDoctor("B", false, "recipe-B")

	docs := env.get("/api/public/doctors")
	if strings.Contains(docs.Body.String(), `"B"`) {
		t.Fatal("unconsented doctor B leaked into public doctors")
	}
	for _, priv := range []string{"phone", "consent", "updated_"} {
		if strings.Contains(docs.Body.String(), priv) {
			t.Fatalf("private field %q leaked", priv)
		}
	}

	recs := env.get("/api/public/recipes")
	if strings.Contains(recs.Body.String(), "recipe-B") {
		t.Fatal("recipe of unconsented doctor B leaked into public recipes")
	}
	if !strings.Contains(recs.Body.String(), "recipe-A") {
		t.Fatal("recipe of consented doctor A missing")
	}
}

func TestPublicHerbsExcludesMergedAliases(t *testing.T) {
	env := newPublicAPI(t)
	canonical := model.Herb{ThaiName: "ขิง"}
	if err := env.g.Create(&canonical).Error; err != nil {
		t.Fatalf("create canonical herb: %v", err)
	}
	alias := model.Herb{ThaiName: "ขิงแก่", AliasOfID: &canonical.ID}
	if err := env.g.Create(&alias).Error; err != nil {
		t.Fatalf("create alias herb: %v", err)
	}

	body := env.get("/api/public/herbs").Body.String()
	if strings.Contains(body, "ขิงแก่") {
		t.Fatalf("public herbs list must not contain merged alias, body = %s", body)
	}
	if !strings.Contains(body, "ขิง") {
		t.Fatalf("public herbs list must contain canonical herb, body = %s", body)
	}
}

func TestPublicHidesPendingDoctorsAndRecipes(t *testing.T) {
	env := newPublicAPI(t)
	env.seedDoctorState(t, "A", true, model.ReviewApproved, "recipe-A")
	env.seedDoctorState(t, "P", true, model.ReviewPending, "recipe-P")

	body := env.get("/api/public/doctors").Body.String()
	if !strings.Contains(body, "A") {
		t.Fatalf("approved doctor A must be visible")
	}
	if strings.Contains(body, `"P"`) {
		t.Fatalf("pending doctor P must be hidden")
	}

	rbody := env.get("/api/public/recipes").Body.String()
	if strings.Contains(rbody, "recipe-P") {
		t.Fatalf("recipe of a pending doctor must be hidden")
	}
	if !strings.Contains(rbody, "recipe-A") {
		t.Fatalf("recipe of an approved doctor must be visible")
	}
}

// TestPublicRecipeReturnsAllPhotosAsArray proves the public recipe
// projection returns every attached photo, as an array in upload order
// (data model §4.5: a recipe may hold many images), not a single string.
func TestPublicRecipeReturnsAllPhotosAsArray(t *testing.T) {
	env := newPublicAPI(t)
	env.seedDoctor("A", true, "recipe-A")
	var rec model.Recipe
	if err := env.g.Where("code = ?", "REC-A").First(&rec).Error; err != nil {
		t.Fatalf("reload recipe: %v", err)
	}
	photos := []model.RecipePhoto{
		{RecipeID: rec.ID, Path: "uploads/one.jpg", SortOrder: 0},
		{RecipeID: rec.ID, Path: "uploads/two.jpg", SortOrder: 1},
	}
	if err := env.g.Create(&photos).Error; err != nil {
		t.Fatalf("create photos: %v", err)
	}

	body := env.get("/api/public/recipes").Body.String()
	wantOrder := `"photos":["uploads/one.jpg","uploads/two.jpg"]`
	if !strings.Contains(body, wantOrder) {
		t.Fatalf("body must contain %s, got %s", wantOrder, body)
	}
}
