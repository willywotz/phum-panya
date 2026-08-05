package export_test

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/export"
	"phum-panya/internal/model"
)

// exportAPI wires an export router with an admin (id 1) and a
// district_editor (id 2, district 1) session, plus one doctor seeded in
// each of two districts, each with a phone set.
type exportAPI struct {
	t           *testing.T
	g           *gorm.DB
	r           *gin.Engine
	adminToken  string
	editorToken string
}

func newExportAPI(t *testing.T) *exportAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	g, err := db.Open(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
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

	doc1 := model.Doctor{
		Code: "MUE-D1", FullName: "Doctor One", DistrictID: d1.ID,
		Phone: "0812345678", Specialty: "herbal", Status: "active", FirstYear: 2560,
	}
	doc2 := model.Doctor{
		Code: "MUE-D2", FullName: "Doctor Two", DistrictID: d2.ID,
		Phone: "0898765432", Specialty: "bone", Status: "active", FirstYear: 2561,
	}
	if err := g.Create(&doc1).Error; err != nil {
		t.Fatalf("create doctor 1: %v", err)
	}
	if err := g.Create(&doc2).Error; err != nil {
		t.Fatalf("create doctor 2: %v", err)
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
	r.Use(auth.LoadUser(store, auth.NewGormUsers(g)))
	export.RegisterRoutes(r, export.NewSource(g))

	return &exportAPI{t: t, g: g, r: r, adminToken: adminToken, editorToken: editorToken}
}

func (env *exportAPI) do(path, token string) *httptest.ResponseRecorder {
	env.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: token})
	}
	rec := httptest.NewRecorder()
	env.r.ServeHTTP(rec, req)
	return rec
}

func TestExportRequiresAuth(t *testing.T) {
	env := newExportAPI(t)
	res := env.do("/api/export/doctors.csv", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated export = %d, want 401", res.Code)
	}
}

func TestEditorExportScopedToOwnDistrictNoPhone(t *testing.T) {
	env := newExportAPI(t)
	res := env.do("/api/export/doctors.csv", env.editorToken)
	if res.Code != http.StatusOK {
		t.Fatalf("editor csv export = %d, want 200, body = %s", res.Code, res.Body.String())
	}
	records, err := csv.NewReader(res.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	for _, col := range records[0] {
		if col == "phone" || strings.HasPrefix(col, "consent") || strings.HasPrefix(col, "updated") {
			t.Fatalf("header contains private column %q", col)
		}
	}
	if len(records) != 2 {
		t.Fatalf("row count = %d, want 2 (header + 1 doctor)", len(records))
	}
	if records[1][1] != "MUE-D1" {
		t.Fatalf("code = %q, want MUE-D1", records[1][1])
	}
}

func TestAdminExportIncludesAllDistricts(t *testing.T) {
	env := newExportAPI(t)
	res := env.do("/api/export/doctors.csv", env.adminToken)
	if res.Code != http.StatusOK {
		t.Fatalf("admin csv export = %d, want 200, body = %s", res.Code, res.Body.String())
	}
	records, err := csv.NewReader(res.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("row count = %d, want 3 (header + 2 doctors)", len(records))
	}
}

func TestEditorExportXLSXOpensAndScoped(t *testing.T) {
	env := newExportAPI(t)
	res := env.do("/api/export/doctors.xlsx", env.editorToken)
	if res.Code != http.StatusOK {
		t.Fatalf("editor xlsx export = %d, want 200, body = %s", res.Code, res.Body.String())
	}
	f, err := excelize.OpenReader(res.Body)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	for _, col := range rows[0] {
		if col == "phone" {
			t.Fatal("header contains phone column")
		}
	}
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2 (header + 1 doctor)", len(rows))
	}
}
