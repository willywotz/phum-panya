package doctor_test

import (
	"bytes"
	stdimage "image"
	"image/jpeg"
	"mime/multipart"
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
	"phum-panya/internal/doctor"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

// doctorAPI wires a doctor router with an admin (id 1) and a district_editor
// (id 2, district 1) session, plus a second district (id 2) neither user
// belongs to.
type doctorAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
	district1   uint
	district2   uint
}

func newDoctorAPI(t *testing.T) *doctorAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "doctor.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	d1 := model.District{Name: "One", Province: "Test"}
	d2 := model.District{Name: "Two", Province: "Test"}
	if err := g.Create(&d1).Error; err != nil {
		t.Fatalf("create district 1: %v", err)
	}
	if err := g.Create(&d2).Error; err != nil {
		t.Fatalf("create district 2: %v", err)
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
		Role: "district_editor", DistrictID: &d1.ID, Active: &active,
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
	mediaStore := &media.Store{Dir: t.TempDir()}
	doctor.RegisterRoutes(r, doctor.NewRepo(g, clock.Real{}, revision.NewRepo(g, clock.Real{})), mediaStore)

	return &doctorAPI{
		t: t, g: g, r: r,
		adminToken: adminToken, editorToken: editorToken,
		district1: d1.ID, district2: d2.ID,
	}
}

// doAsEditor performs a request as the district-1 editor and returns the
// recorded response.
func (env *doctorAPI) doAsEditor(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.editorToken, body)
}

// doAsAdmin performs a request as the central admin and returns the
// recorded response.
func (env *doctorAPI) doAsAdmin(method, path, body string) *httptest.ResponseRecorder {
	return env.do(method, path, env.adminToken, body)
}

func (env *doctorAPI) do(method, path, token, body string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestEditorCannotWriteOtherDistrict(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"code":"MUE-01","full_name":"H","district_id":2,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", body)
	if res.Code != http.StatusForbidden {
		t.Fatalf("cross-district create = %d, want 403", res.Code)
	}
}

func TestCreateSetsAuditAndConsentDefaultsFalse(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"code":"MUE-02","full_name":"H2","district_id":1,"status":"active","first_year":2565,"specialty":["bone"]}`
	res := env.doAsEditor("POST", "/api/doctors", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", res.Code, res.Body.String())
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-02").First(&d)
	if d.ConsentObtained {
		t.Fatal("consent should default false")
	}
	if d.UpdatedBy == nil || *d.UpdatedBy != 2 || d.UpdatedAt.IsZero() {
		t.Fatalf("audit not set: %+v", d)
	}
}

func TestAdminCanWriteAnyDistrict(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"code":"MUE-03","full_name":"H3","district_id":2,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsAdmin("POST", "/api/doctors", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("admin create in district 2 = %d, want 201, body = %s", res.Code, res.Body.String())
	}
}

// TestCreateMissingCodeReturns400 proves doctorRequest rejects a create body
// missing the required code field at bind time, not a 500 from a later
// unique-index or CHECK failure.
func TestCreateMissingCodeReturns400(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"full_name":"H","district_id":1,"status":"active","first_year":2565}`
	res := env.doAsEditor("POST", "/api/doctors", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}
}

// TestCreateInvalidStatusReturns400 proves an out-of-range status is a clean
// 400 from binding, not a 500 from the chk_doctor_status DB check.
func TestCreateInvalidStatusReturns400(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"code":"MUE-10","full_name":"H","district_id":1,"status":"bogus","first_year":2565}`
	res := env.doAsEditor("POST", "/api/doctors", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}
}

// TestCreateInvalidGenderReturns400 proves an out-of-range gender is a clean
// 400 from binding, not a 500 from the chk_doctor_gender DB check.
func TestCreateInvalidGenderReturns400(t *testing.T) {
	env := newDoctorAPI(t)
	body := `{"code":"MUE-11","full_name":"H","district_id":1,"status":"active","gender":"bogus","first_year":2565}`
	res := env.doAsEditor("POST", "/api/doctors", body)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400, body %s", res.Code, res.Body.String())
	}
}

func TestListRequiresDistrictQuery(t *testing.T) {
	env := newDoctorAPI(t)
	res := env.doAsAdmin("GET", "/api/doctors", "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing district_id = %d, want 400", res.Code)
	}

	res = env.doAsAdmin("GET", "/api/doctors?district_id=1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("with district_id = %d, want 200, body = %s", res.Code, res.Body.String())
	}
}

func TestUpdateCrossDistrictForbidden(t *testing.T) {
	env := newDoctorAPI(t)
	create := `{"code":"MUE-04","full_name":"H4","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-04").First(&d)

	move := `{"code":"MUE-04","full_name":"H4","district_id":2,"status":"active","first_year":2565,"specialty":["herbal"]}`
	path := "/api/doctors/" + strconv.FormatUint(uint64(d.ID), 10)
	res = env.doAsEditor("PUT", path, move)
	if res.Code != http.StatusForbidden {
		t.Fatalf("move to other district = %d, want 403", res.Code)
	}
}

func TestDeleteEnforcesOwnDistrict(t *testing.T) {
	env := newDoctorAPI(t)
	create := `{"code":"MUE-05","full_name":"H5","district_id":2,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsAdmin("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-05").First(&d)

	path := "/api/doctors/" + strconv.FormatUint(uint64(d.ID), 10)
	res = env.doAsEditor("DELETE", path, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("editor delete other district = %d, want 403", res.Code)
	}
	res = env.doAsAdmin("DELETE", path, "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("admin delete = %d, want 204", res.Code)
	}
}

// TestEditorPhotoUploadStagesPendingPhoto proves an editor's photo upload
// over HTTP stages the path in pending_photo rather than publishing it: the
// live photo stays unset until a central admin approves it (P2).
func TestEditorPhotoUploadStagesPendingPhoto(t *testing.T) {
	env := newDoctorAPI(t)
	create := `{"code":"MUE-06","full_name":"H6","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-06").First(&d)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("photo", "photo.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if err := jpeg.Encode(part, stdimage.NewNRGBA(stdimage.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatalf("encode photo: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	path := "/api/doctors/" + strconv.FormatUint(uint64(d.ID), 10) + "/photo"
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: env.editorToken})
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("photo upload = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var reloaded model.Doctor
	env.g.First(&reloaded, d.ID)
	if reloaded.Photo != "" {
		t.Fatalf("live photo must stay unset until approval, got %q", reloaded.Photo)
	}
	if reloaded.PendingPhoto == nil || *reloaded.PendingPhoto == "" {
		t.Fatal("pending_photo was not staged")
	}
}

// TestUpdatePreservesPhoto proves that editing a doctor after its photo was
// uploaded does not wipe the photo: the frontend PUT body carries no photo
// field, and Update must not blank the stored path.
func TestUpdatePreservesPhoto(t *testing.T) {
	env := newDoctorAPI(t)
	create := `{"code":"MUE-07","full_name":"H7","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-07").First(&d)

	repo := doctor.NewRepo(env.g, clock.Real{}, revision.NewRepo(env.g, clock.Real{}))
	if err := repo.SetPhoto(d.ID, 1, "uploads/h7.jpg", true); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	path := "/api/doctors/" + strconv.FormatUint(uint64(d.ID), 10)
	edit := `{"code":"MUE-07","full_name":"H7","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"],"consent_obtained":true}`
	res = env.doAsEditor("PUT", path, edit)
	if res.Code != http.StatusOK {
		t.Fatalf("edit = %d, want 200, body = %s", res.Code, res.Body.String())
	}

	var reloaded model.Doctor
	env.g.First(&reloaded, d.ID)
	if reloaded.Photo != "uploads/h7.jpg" {
		t.Fatalf("Photo = %q, want unchanged %q", reloaded.Photo, "uploads/h7.jpg")
	}
}

// TestEditorCreateGoesPendingAdminIsImmediate proves the doctor write path
// branches on role: an editor create enters the pending queue with no
// revision logged yet, while an admin create publishes immediately and is
// logged as one revision.
func TestEditorCreateGoesPendingAdminIsImmediate(t *testing.T) {
	env := newDoctorAPI(t)

	body := `{"code":"D9","full_name":"เจ้าของ","specialty":["ยาต้ม"],"status":"active","first_year":2568,"district_id":1}`
	rec := env.doAsEditor("POST", "/api/doctors", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("editor create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created model.Doctor
	env.g.Where("code = ?", "D9").First(&created)
	if created.ReviewState != model.ReviewPending {
		t.Fatalf("editor create review_state = %q, want pending", created.ReviewState)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", created.ID).Count(&revCount)
	if revCount != 0 {
		t.Fatalf("editor create should not append a revision yet, got %d", revCount)
	}

	body2 := `{"code":"D10","full_name":"แอดมิน","specialty":["ยาต้ม"],"status":"active","first_year":2568,"district_id":1}`
	rec2 := env.doAsAdmin("POST", "/api/doctors", body2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("admin create status = %d", rec2.Code)
	}
	var adminDoc model.Doctor
	env.g.Where("code = ?", "D10").First(&adminDoc)
	if adminDoc.ReviewState != model.ReviewApproved {
		t.Fatalf("admin create review_state = %q, want approved", adminDoc.ReviewState)
	}
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", adminDoc.ID).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("admin create revisions = %d, want 1", revCount)
	}
}

// TestEditorPhotoChangeGoesPending proves an editor photo upload does not
// publish immediately: it stages the path in pending_photo, leaves the live
// photo untouched, and still logs a revision for the proposal.
func TestEditorPhotoChangeGoesPending(t *testing.T) {
	env := newDoctorAPI(t)
	// An editor create enters the pending queue and logs no revision (see
	// TestEditorCreateGoesPendingAdminIsImmediate), so the only revision
	// counted below comes from the photo change itself.
	create := `{"code":"MUE-08","full_name":"H8","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-08").First(&d)
	env.g.Model(&model.Doctor{}).Where("id = ?", d.ID).Update("photo", "uploads/original.jpg")

	repo := doctor.NewRepo(env.g, clock.Real{}, revision.NewRepo(env.g, clock.Real{}))
	if err := repo.SetPhoto(d.ID, 2, "uploads/proposed.jpg", false); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	var reloaded model.Doctor
	env.g.First(&reloaded, d.ID)
	if reloaded.Photo != "uploads/original.jpg" {
		t.Fatalf("Photo = %q, want unchanged %q", reloaded.Photo, "uploads/original.jpg")
	}
	if reloaded.PendingPhoto == nil || *reloaded.PendingPhoto != "uploads/proposed.jpg" {
		t.Fatalf("PendingPhoto = %v, want uploads/proposed.jpg", reloaded.PendingPhoto)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", d.ID).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("editor photo change revisions = %d, want 1", revCount)
	}
}

// TestAdminPhotoChangeIsImmediate proves an admin photo upload bypasses
// approval: it writes the live photo column right away and logs a revision.
func TestAdminPhotoChangeIsImmediate(t *testing.T) {
	env := newDoctorAPI(t)
	// Seeded via the editor so its create logs no revision (see
	// TestEditorCreateGoesPendingAdminIsImmediate); the only revision
	// counted below comes from the admin's photo change.
	create := `{"code":"MUE-09","full_name":"H9","district_id":1,"status":"active","first_year":2565,"specialty":["herbal"]}`
	res := env.doAsEditor("POST", "/api/doctors", create)
	if res.Code != http.StatusCreated {
		t.Fatalf("setup create = %d", res.Code)
	}
	var d model.Doctor
	env.g.Where("code = ?", "MUE-09").First(&d)

	repo := doctor.NewRepo(env.g, clock.Real{}, revision.NewRepo(env.g, clock.Real{}))
	if err := repo.SetPhoto(d.ID, 1, "uploads/admin.jpg", true); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	var reloaded model.Doctor
	env.g.First(&reloaded, d.ID)
	if reloaded.Photo != "uploads/admin.jpg" {
		t.Fatalf("Photo = %q, want uploads/admin.jpg", reloaded.Photo)
	}
	if reloaded.PendingPhoto != nil {
		t.Fatalf("PendingPhoto = %v, want nil", reloaded.PendingPhoto)
	}
	var revCount int64
	env.g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ?", "doctor", d.ID).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("admin photo change revisions = %d, want 1", revCount)
	}
}
