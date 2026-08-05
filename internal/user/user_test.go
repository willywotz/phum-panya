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
	store      *auth.SessionStore
	adminToken string
}

func (env *userAPI) do(method, path, body string) *httptest.ResponseRecorder {
	return env.doAs(env.adminToken, method, path, body)
}

func (env *userAPI) doAs(token, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
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
	if err := db.AutoMigrate(g); err != nil {
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
	engine.Use(auth.LoadUser(store, auth.NewGormUsers(g)))
	user.RegisterRoutes(engine, user.NewRepo(g))

	return &userAPI{g: g, engine: engine, store: store, adminToken: adminToken}
}

// newEditorSession seeds a district_editor for districtID and returns a
// session token for them, built the same way newUserAPI builds the admin's.
func (env *userAPI) newEditorSession(t *testing.T, districtID uint) string {
	t.Helper()
	active := true
	editor := model.User{
		FullName: "Editor", Email: "editor@x", PasswordHash: "hash",
		Role: "district_editor", DistrictID: &districtID, Active: &active,
	}
	if err := env.g.Create(&editor).Error; err != nil {
		t.Fatalf("create editor: %v", err)
	}
	token, err := env.store.Create(editor.ID)
	if err != nil {
		t.Fatalf("create editor session: %v", err)
	}
	return token
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

// TestCreateUserMissingPasswordReturns400 proves createUserRequest rejects
// a create body missing the required password field at bind time, instead
// of silently hashing an empty string into a usable login.
func TestCreateUserMissingPasswordReturns400(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users",
		`{"full_name":"Ed","email":"ed@x","role":"district_editor","district_id":1}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", res.Code, res.Body.String())
	}
}

// TestCreateUserInvalidRoleReturns400 proves an out-of-range role is
// rejected at bind time via the oneof tag.
func TestCreateUserInvalidRoleReturns400(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users",
		`{"full_name":"Ed","email":"ed@x","password":"pw123456","role":"bogus"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", res.Code, res.Body.String())
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

func TestSetActiveMissingIDReturns404(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users/999/active", `{"active":false}`)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", res.Code, res.Body.String())
	}
}

func TestSetPasswordMissingIDReturns404(t *testing.T) {
	env := newUserAPI(t)
	res := env.do("POST", "/api/users/999/password", `{"password":"abcdefgh"}`)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", res.Code, res.Body.String())
	}
}

func TestListRequiresCentralAdmin(t *testing.T) {
	env := newUserAPI(t)
	editorToken := env.newEditorSession(t, 1)

	res := env.doAs(editorToken, "GET", "/api/users", "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", res.Code, res.Body.String())
	}
}
