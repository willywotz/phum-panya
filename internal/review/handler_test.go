package review_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/review"
	"phum-panya/internal/revision"
)

// reviewAPI wires a review router with an admin (central_admin) and a
// district-editor session, mirroring internal/doctor/doctor_test.go's
// newDoctorAPI.
type reviewAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
	districtID  uint
}

func newReviewAPI(t *testing.T) *reviewAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	dist := model.District{Name: "One", Province: "Test"}
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

	r := gin.New()
	r.Use(auth.LoadUser(store, g))
	review.RegisterRoutes(r, review.NewRepo(g, revision.NewRepo(g, clock.Real{})))

	return &reviewAPI{
		t: t, g: g, r: r,
		adminToken: adminToken, editorToken: editorToken,
		districtID: dist.ID,
	}
}

func (env *reviewAPI) doAsEditor(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editorToken, body)
}

func (env *reviewAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
}

func (env *reviewAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestQueueRequiresCentralAdmin(t *testing.T) {
	env := newReviewAPI(t)
	if rec := env.doAsEditor("GET", "/api/review/queue", ""); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	if rec := env.doAsAdmin("GET", "/api/review/queue", ""); rec.Code != 200 {
		t.Fatalf("admin must be allowed, got %d", rec.Code)
	}
}

func TestApproveEndpointPromotes(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewPending}
	env.g.Create(&d)
	path := fmt.Sprintf("/api/review/entry/doctor/%d/approve", d.ID)
	rec := env.doAsAdmin("POST", path, "")
	if rec.Code != 200 {
		t.Fatalf("approve status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got model.Doctor
	env.g.First(&got, d.ID)
	if got.ReviewState != model.ReviewApproved {
		t.Fatalf("state = %q, want approved", got.ReviewState)
	}
}
