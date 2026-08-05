package herb_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/herb"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
)

// herbAPI wires a herb router with an admin (central_admin) and a
// district-editor session, mirroring internal/review/handler_test.go's
// newReviewAPI.
type herbAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
	districtID  uint
}

func newHerbAPI(t *testing.T) *herbAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g := newDB(t)

	dist := model.District{Name: "One", Province: "Test"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("create district: %v", err)
	}

	active := true
	admin := model.User{
		FullName: "Admin", Email: "admin@x", PasswordHash: "hash",
		Role: model.RoleCentralAdmin, Active: &active,
	}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	editor := model.User{
		FullName: "Editor", Email: "editor@x", PasswordHash: "hash",
		Role: model.RoleDistrictEditor, DistrictID: &dist.ID, Active: &active,
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
	r.Use(auth.LoadUser(store, auth.NewGormUsers(g)))
	herb.RegisterRoutes(r, herb.NewRepo(g), &media.LocalStore{Dir: t.TempDir()})

	return &herbAPI{
		t: t, g: g, r: r,
		adminToken: adminToken, editorToken: editorToken,
		districtID: dist.ID,
	}
}

func (env *herbAPI) doAsEditor(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editorToken, body)
}

func (env *herbAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
}

func (env *herbAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

// TestCreateHerbMissingThaiNameReturns400 proves herbRequest rejects a
// create body missing the required thai_name field at bind time.
func TestCreateHerbMissingThaiNameReturns400(t *testing.T) {
	env := newHerbAPI(t)
	rec := env.doAsEditor("POST", "/api/herbs", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestEditorMayCreateHerbButNotMerge(t *testing.T) {
	env := newHerbAPI(t)

	rec := env.doAsEditor("POST", "/api/herbs", `{"thai_name":"กระชาย"}`)
	if rec.Code != 201 {
		t.Fatalf("editor create herb status = %d, body=%s", rec.Code, rec.Body.String())
	}
	// the created herb is stamped with the editor's district
	var h model.Herb
	if err := env.g.Where("thai_name = ?", "กระชาย").First(&h).Error; err != nil {
		t.Fatalf("herb not created: %v", err)
	}
	if h.CreatedByDistrictID == nil {
		t.Fatalf("editor-created herb must carry a district provenance")
	}

	// editor may NOT merge (admin-only)
	if rec := env.doAsEditor("POST", "/api/herbs/2/merge/1", ""); rec.Code != 403 {
		t.Fatalf("editor merge must be forbidden, got %d", rec.Code)
	}
}

func TestEditorCannotEditAnotherDistrictHerb(t *testing.T) {
	env := newHerbAPI(t)
	// a herb created by some OTHER district
	other := uint(999)
	h := model.Herb{ThaiName: "ขมิ้น", CreatedByDistrictID: &other}
	env.g.Create(&h)

	rec := env.doAsEditor("PUT", "/api/herbs/"+itoa(h.ID), `{"thai_name":"ขมิ้นชัน"}`)
	if rec.Code != 403 {
		t.Fatalf("editing another district's herb must be 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminMayMergeHerbs(t *testing.T) {
	env := newHerbAPI(t)
	canonical := model.Herb{ThaiName: "ขิง"}
	dup := model.Herb{ThaiName: "ขิง (ซ้ำ)"}
	env.g.Create(&canonical)
	env.g.Create(&dup)

	path := "/api/herbs/" + itoa(dup.ID) + "/merge/" + itoa(canonical.ID)
	rec := env.doAsAdmin("POST", path, "")
	if rec.Code != 200 {
		t.Fatalf("admin merge status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var alias model.Herb
	env.g.First(&alias, dup.ID)
	if alias.AliasOfID == nil || *alias.AliasOfID != canonical.ID {
		t.Fatalf("dup must be marked alias of canonical")
	}
}

func TestMergeHandlerMapsErrorsToStatus(t *testing.T) {
	env := newHerbAPI(t)
	canonical := model.Herb{ThaiName: "ก"}
	dup := model.Herb{ThaiName: "ข"}
	env.g.Create(&canonical)
	env.g.Create(&dup)
	aliasOfCanonical := model.Herb{ThaiName: "ค", AliasOfID: &canonical.ID}
	env.g.Create(&aliasOfCanonical)

	cases := []struct {
		name string
		path string
		want int
	}{
		{"self merge", "/api/herbs/" + itoa(canonical.ID) + "/merge/" + itoa(canonical.ID), 400},
		{"missing canonical", "/api/herbs/" + itoa(dup.ID) + "/merge/99999", 404},
		{"chained merge", "/api/herbs/" + itoa(dup.ID) + "/merge/" + itoa(aliasOfCanonical.ID), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := env.doAsAdmin("POST", tc.path, ""); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestNearDuplicatesEndpointRequiresAuth(t *testing.T) {
	env := newHerbAPI(t)
	env.g.Create(&model.Herb{ThaiName: "ขิง"})

	rec := env.doAsEditor("GET", "/api/herbs/near-duplicates?thaiName=ขิ", "")
	if rec.Code != 200 {
		t.Fatalf("near-duplicates status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ขิง") {
		t.Fatalf("near-duplicates body = %s, want ขิง", rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/herbs/near-duplicates?thaiName=ขิ", nil)
	rec = httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("unauthenticated near-duplicates status = %d, want 401", rec.Code)
	}
}
