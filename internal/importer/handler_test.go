package importer_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/doctor"
	"phum-panya/internal/herb"
	"phum-panya/internal/importer"
	"phum-panya/internal/model"
	"phum-panya/internal/recipe"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
)

// importerAPI wires the importer router with an admin (central_admin) and a
// district-editor session, mirroring internal/review/handler_test.go's
// newReviewAPI, over the internal/importer/importer_test.go newImporterEnv
// domain wiring (all repos + a seeded district id 1).
type importerAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
	districtID  uint
}

func newImporterAPI(t *testing.T) *importerAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "importer_api.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	dist := model.District{Name: "d", Province: "p"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("create district: %v", err)
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
		Role: "district_editor", DistrictID: &dist.ID, Active: &active,
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

	rev := revision.NewRepo(g, clock.Real{})
	lock := yearlock.NewRepo(g, clock.Real{})
	im := importer.NewImporter(
		g, clock.Real{},
		doctor.NewRepo(g, clock.Real{}, rev),
		recipe.NewRepo(g, clock.Real{}, rev, lock),
		caserec.NewRepo(g, clock.Real{}, rev, lock),
		herb.NewRepo(g),
		lock,
	)

	r := gin.New()
	r.MaxMultipartMemory = 8 << 20
	r.Use(auth.LoadUser(store, g))
	importer.RegisterRoutes(r, im)

	return &importerAPI{
		t: t, g: g, r: r,
		adminToken: adminToken, editorToken: editorToken,
		districtID: dist.ID,
	}
}

func (env *importerAPI) doMultipart(t *testing.T, token, method, path string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func multipartWorkbook(t *testing.T, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "import.xlsx")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func TestImportEndpointAdminOnlyMultipart(t *testing.T) {
	env := newImporterAPI(t)

	body, ct := multipartWorkbook(t, buildFixtureWorkbook(t))
	if rec := env.doMultipart(t, env.editorToken, "POST", "/api/imports?dryRun=true", body, ct); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}

	body2, ct2 := multipartWorkbook(t, buildFixtureWorkbook(t))
	rec := env.doMultipart(t, env.adminToken, "POST", "/api/imports?dryRun=true", body2, ct2)
	if rec.Code != 200 {
		t.Fatalf("admin dry run status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "doctorsNew") {
		t.Fatalf("dry-run response should contain the report, got %s", rec.Body.String())
	}
}
