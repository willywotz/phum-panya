package district_test

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
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/district"
	"phum-panya/internal/model"
)

// newDB opens a fresh temp SQLite DB and migrates it. It is a local copy of
// auth_test's helper: that one lives in package auth_test and cannot be
// imported from here.
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "district.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return g
}

// seedUsers creates a central_admin (id 1) and a district_editor (id 2)
// belonging to districtID.
func seedUsers(t *testing.T, g *gorm.DB, districtID uint) {
	t.Helper()
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
		Role: "district_editor", DistrictID: &districtID, Active: &active,
	}
	if err := g.Create(&editor).Error; err != nil {
		t.Fatalf("create editor: %v", err)
	}
}

func TestRepoCreateAndList(t *testing.T) {
	g := newDB(t)
	repo := district.NewRepo(g)

	d := &model.District{Name: "Mueang", Province: "Chiang Mai"}
	if err := repo.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == 0 {
		t.Fatal("Create did not set ID")
	}

	got, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List len = %d, want 1", len(got))
	}
	if got[0].Name != "Mueang" || got[0].Province != "Chiang Mai" {
		t.Fatalf("List[0] = %+v, want Name=Mueang Province=Chiang Mai", got[0])
	}
}

// router seeds one district, an admin, and an editor for that district, then
// wires up the full LoadUser + district routes chain.
func router(t *testing.T) (r *gin.Engine, adminToken, editorToken string, districtID uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := newDB(t)

	seed := model.District{Name: "Seed", Province: "Test"}
	if err := g.Create(&seed).Error; err != nil {
		t.Fatalf("create seed district: %v", err)
	}
	seedUsers(t, g, seed.ID)

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(1)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}
	editorToken, err = store.Create(2)
	if err != nil {
		t.Fatalf("Create editor session: %v", err)
	}

	r = gin.New()
	r.Use(auth.LoadUser(store, g))
	district.RegisterRoutes(r, district.NewRepo(g))

	return r, adminToken, editorToken, seed.ID
}

func withCookie(req *http.Request, token string) *http.Request {
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	return req
}

func TestListRequiresAuth(t *testing.T) {
	r, adminToken, _, _ := router(t)

	req := httptest.NewRequest(http.MethodGet, "/api/districts", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie: status = %d, want 401", rec.Code)
	}

	req = withCookie(httptest.NewRequest(http.MethodGet, "/api/districts", nil), adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin cookie: status = %d, want 200", rec.Code)
	}
}

func TestPostRequiresCentralAdmin(t *testing.T) {
	r, adminToken, editorToken, _ := router(t)
	body := `{"name":"New","province":"Prov"}`

	req := withCookie(httptest.NewRequest(http.MethodPost, "/api/districts", strings.NewReader(body)), editorToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor: status = %d, want 403", rec.Code)
	}

	req = withCookie(httptest.NewRequest(http.MethodPost, "/api/districts", strings.NewReader(body)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin: status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"New"`) {
		t.Fatalf("body = %s, want created district", rec.Body.String())
	}
}

// TestPostMissingProvinceReturns400 proves districtRequest rejects a create
// body missing the required province field at bind time.
func TestPostMissingProvinceReturns400(t *testing.T) {
	r, adminToken, _, _ := router(t)
	body := `{"name":"New"}`

	req := withCookie(httptest.NewRequest(http.MethodPost, "/api/districts", strings.NewReader(body)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPutUpdatesDistrict(t *testing.T) {
	r, adminToken, _, districtID := router(t)
	body := `{"name":"Renamed","province":"NewProv"}`

	path := "/api/districts/" + itoa(districtID)
	req := withCookie(httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Renamed") {
		t.Fatalf("body = %s, want Renamed", rec.Body.String())
	}
}

func TestPutMissingIDReturns404(t *testing.T) {
	r, adminToken, _, _ := router(t)
	body := `{"name":"X","province":"Y"}`

	req := withCookie(httptest.NewRequest(http.MethodPut, "/api/districts/999", strings.NewReader(body)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPutBadIDReturns400(t *testing.T) {
	r, adminToken, _, _ := router(t)
	body := `{"name":"X","province":"Y"}`

	req := withCookie(httptest.NewRequest(http.MethodPut, "/api/districts/abc", strings.NewReader(body)), adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDeleteRemovesDistrict(t *testing.T) {
	r, adminToken, _, districtID := router(t)

	path := "/api/districts/" + itoa(districtID)
	req := withCookie(httptest.NewRequest(http.MethodDelete, path, nil), adminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	req = withCookie(httptest.NewRequest(http.MethodGet, "/api/districts", nil), adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Seed") {
		t.Fatalf("body still contains deleted district: %s", rec.Body.String())
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
