package yearlock_test

import (
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
	"phum-panya/internal/yearlock"
)

// yearLockAPI wires a yearlock router with an admin (central_admin) and a
// district-editor session, mirroring internal/review/handler_test.go's
// newReviewAPI.
type yearLockAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
	districtID  uint
}

func newYearLockAPI(t *testing.T) *yearLockAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "yearlock.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
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
	yearlock.RegisterRoutes(r, yearlock.NewRepo(g, clock.Real{}))

	return &yearLockAPI{
		t: t, g: g, r: r,
		adminToken: adminToken, editorToken: editorToken,
		districtID: dist.ID,
	}
}

func (env *yearLockAPI) doAsEditor(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editorToken, body)
}

func (env *yearLockAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
}

func (env *yearLockAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestLockEndpointAdminOnly(t *testing.T) {
	env := newYearLockAPI(t)
	if rec := env.doAsEditor("POST", "/api/year-locks", `{"dataYear":2567}`); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	if rec := env.doAsAdmin("POST", "/api/year-locks", `{"dataYear":2567}`); rec.Code != 201 {
		t.Fatalf("admin lock status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var n int64
	env.g.Model(&model.YearLock{}).Where("data_year = ?", 2567).Count(&n)
	if n != 1 {
		t.Fatalf("admin lock must create a year_lock row, got %d", n)
	}
}

func TestUnlockEndpoint(t *testing.T) {
	env := newYearLockAPI(t)
	env.g.Create(&model.YearLock{DataYear: 2566, LockedBy: 1})
	rec := env.doAsAdmin("DELETE", "/api/year-locks/2566", "")
	if rec.Code != 200 {
		t.Fatalf("unlock status = %d", rec.Code)
	}
	var n int64
	env.g.Model(&model.YearLock{}).Where("data_year = ?", 2566).Count(&n)
	if n != 0 {
		t.Fatalf("unlock must remove the row")
	}
}
