package user_test

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
	"phum-panya/internal/user"
)

// userAPI bundles a routed engine, its DB, and a request helper that
// attaches the seeded central_admin's session cookie.
type userAPI struct {
	g          *gorm.DB
	engine     *gin.Engine
	adminToken string
}

func (env *userAPI) do(method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.AddCookie(&http.Cookie{Name: "session", Value: env.adminToken})
	rec := httptest.NewRecorder()
	env.engine.ServeHTTP(rec, r)
	return rec
}

// newUserAPI opens a fresh temp DB, seeds a district and a central_admin
// (id 1), and wires the user routes behind auth.LoadUser.
func newUserAPI(t *testing.T) *userAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	d := model.District{Name: "Seed", Province: "Test"}
	if err := g.Create(&d).Error; err != nil {
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

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(admin.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	engine := gin.New()
	engine.Use(auth.LoadUser(store, g))
	user.RegisterRoutes(engine, user.NewRepo(g))

	return &userAPI{g: g, engine: engine, adminToken: adminToken}
}

func TestUserResponseHidesHash(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users",
		`{"full_name":"Ed","email":"ed@x","password":"pw123456","role":"district_editor","district_id":1}`)
	if res.Code != 201 {
		t.Fatalf("create status %d", res.Code)
	}

	list := env.do("GET", "/api/users", "")
	if strings.Contains(list.Body.String(), "password") || strings.Contains(list.Body.String(), "$2") {
		t.Fatalf("hash leaked: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "ed@x") {
		t.Fatal("created user missing from list")
	}

	env.do("POST", "/api/users/2/password", `{"password":"newpw999"}`)
	var u model.User
	env.g.Where("email = ?", "ed@x").First(&u)
	if !auth.CheckPassword(u.PasswordHash, "newpw999") {
		t.Fatal("reset password does not verify")
	}
}

func TestCreateDistrictEditorRequiresDistrictID(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users",
		`{"full_name":"Ed","email":"ed@x","password":"pw123456","role":"district_editor"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", res.Code, res.Body.String())
	}
}

func TestSetActivePersistsFalse(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users",
		`{"full_name":"Ed","email":"ed@x","password":"pw123456","role":"district_editor","district_id":1}`)
	if res.Code != 201 {
		t.Fatalf("create status %d", res.Code)
	}

	res = env.do("POST", "/api/users/2/active", `{"active":false}`)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", res.Code, res.Body.String())
	}

	var u model.User
	if err := env.g.First(&u, 2).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if u.Active == nil || *u.Active {
		t.Fatalf("Active = %v, want false", u.Active)
	}
}
