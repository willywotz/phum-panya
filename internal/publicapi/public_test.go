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
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	r := gin.New()
	publicapi.RegisterRoutes(r, g)

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
