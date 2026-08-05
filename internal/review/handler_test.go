package review_test

import (
	"encoding/json"
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

func TestDetailEndpointRequiresCentralAdmin(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D1", Photo: "-", FullName: "หมอ ก", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewPending}
	env.g.Create(&d)
	path := fmt.Sprintf("/api/review/entry/doctor/%d", d.ID)
	if rec := env.doAsEditor("GET", path, ""); rec.Code != 403 {
		t.Fatalf("editor must be forbidden, got %d", rec.Code)
	}
	if rec := env.doAsAdmin("GET", path, ""); rec.Code != 200 {
		t.Fatalf("admin must be allowed, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDetailReportsCreateShape(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D2", Photo: "-", FullName: "หมอ ข", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewPending}
	env.g.Create(&d)
	rec := env.doAsAdmin("GET", fmt.Sprintf("/api/review/entry/doctor/%d", d.ID), "")
	var got struct {
		Action   string          `json:"action"`
		Identity string          `json:"identity"`
		DoctorID uint            `json:"doctorId"`
		Current  json.RawMessage `json:"current"`
		Proposed json.RawMessage `json:"proposed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != model.ActionCreate || got.Identity != "หมอ ข" || got.DoctorID != d.ID {
		t.Fatalf("bad detail: %+v", got)
	}
	if string(got.Current) != "null" {
		t.Fatalf("create current must be null, got %s", got.Current)
	}
}

func TestDetailReportsUpdateShape(t *testing.T) {
	env := newReviewAPI(t)
	pending := `{"FullName":"หมอ ใหม่"}`
	d := model.Doctor{Code: "D3", Photo: "-", FullName: "หมอ เดิม", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewApproved, PendingJSON: &pending}
	env.g.Create(&d)
	rec := env.doAsAdmin("GET", fmt.Sprintf("/api/review/entry/doctor/%d", d.ID), "")
	var got struct {
		Action   string          `json:"action"`
		Proposed json.RawMessage `json:"proposed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Action != model.ActionUpdate {
		t.Fatalf("action = %q, want update", got.Action)
	}
	if !strings.Contains(string(got.Proposed), "หมอ ใหม่") {
		t.Fatalf("proposed must carry pending overlay, got %s", got.Proposed)
	}
}

func TestDetailReportsDeleteShape(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D4", Photo: "-", FullName: "หมอ ค", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewApproved, PendingDelete: true}
	env.g.Create(&d)
	body := env.doAsAdmin("GET", fmt.Sprintf("/api/review/entry/doctor/%d", d.ID), "")
	var got struct {
		Action   string          `json:"action"`
		Current  json.RawMessage `json:"current"`
		Proposed json.RawMessage `json:"proposed"`
	}
	if err := json.Unmarshal(body.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != model.ActionDelete {
		t.Fatalf("action = %q, want delete", got.Action)
	}
	if string(got.Current) == "null" {
		t.Fatalf("delete current must be the live row, got %s", got.Current)
	}
	if string(got.Proposed) != "null" {
		t.Fatalf("delete proposed must be null, got %s", got.Proposed)
	}
}

func TestDetailReportsCaseShapeWithParentDoctorID(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D5", Photo: "-", FullName: "หมอ ง", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewApproved}
	env.g.Create(&d)
	rp := model.Recipe{Code: "R5", Name: "ยา ง", DoctorID: d.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568, ReviewState: model.ReviewApproved}
	env.g.Create(&rp)
	c := model.Case{RecipeID: rp.ID, Condition: "ไข้", Result: "-", DataYear: 2568, ReviewState: model.ReviewPending}
	env.g.Create(&c)

	body := env.doAsAdmin("GET", fmt.Sprintf("/api/review/entry/case/%d", c.ID), "")
	var got struct {
		Action   string `json:"action"`
		Identity string `json:"identity"`
		DoctorID uint   `json:"doctorId"`
	}
	if err := json.Unmarshal(body.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != model.ActionCreate {
		t.Fatalf("action = %q, want create", got.Action)
	}
	if got.Identity != "ไข้" {
		t.Fatalf("identity = %q, want the case condition", got.Identity)
	}
	if got.DoctorID != d.ID {
		t.Fatalf("doctorId = %d, want parent doctor id %d", got.DoctorID, d.ID)
	}
}

func TestDetailReportsRecipeUpdateShapeWithIngredients(t *testing.T) {
	env := newReviewAPI(t)
	d := model.Doctor{Code: "D6", Photo: "-", FullName: "หมอ จ", Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: env.districtID, ReviewState: model.ReviewApproved}
	env.g.Create(&d)
	h := model.Herb{ThaiName: "ขิง"}
	env.g.Create(&h)
	rp := model.Recipe{Code: "R6", Name: "ยา จ", DoctorID: d.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568, ReviewState: model.ReviewApproved}
	env.g.Create(&rp)

	pending := fmt.Sprintf(`{"recipe":{"ID":%d,"Code":"R6","Name":"ยา จ ใหม่","DoctorID":%d,"Indication":"-","Preparation":"-","Usage":"-","DataYear":2568,"ReviewState":"approved"},"ingredients":[{"RecipeID":%d,"HerbID":%d,"Amount":"3","Unit":"g"}]}`, rp.ID, d.ID, rp.ID, h.ID)
	env.g.Model(&model.Recipe{}).Where("id = ?", rp.ID).Update("pending_json", pending)

	body := env.doAsAdmin("GET", fmt.Sprintf("/api/review/entry/recipe/%d", rp.ID), "")
	var got struct {
		Action   string          `json:"action"`
		Identity string          `json:"identity"`
		DoctorID uint            `json:"doctorId"`
		Proposed json.RawMessage `json:"proposed"`
	}
	if err := json.Unmarshal(body.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != model.ActionUpdate {
		t.Fatalf("action = %q, want update", got.Action)
	}
	if got.Identity != "ยา จ" {
		t.Fatalf("identity = %q, want the current recipe name", got.Identity)
	}
	if got.DoctorID != d.ID {
		t.Fatalf("doctorId = %d, want owning doctor id %d", got.DoctorID, d.ID)
	}
	if !strings.Contains(string(got.Proposed), `"Amount":"3"`) {
		t.Fatalf("proposed must keep the ingredients array intact, got %s", got.Proposed)
	}
}

func TestDetailRejectsUnknownEntityType(t *testing.T) {
	env := newReviewAPI(t)
	rec := env.doAsAdmin("GET", "/api/review/entry/widget/1", "")
	if rec.Code != 400 {
		t.Fatalf("unknown entity type status = %d, want 400", rec.Code)
	}
}
