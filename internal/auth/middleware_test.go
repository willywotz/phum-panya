package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := newDB(t)

	active := true
	districtID := uint(1)
	editor := model.User{
		FullName:     "Editor",
		Email:        "editor@x",
		PasswordHash: "hash",
		Role:         "district_editor",
		DistrictID:   &districtID,
		Active:       &active,
	}
	if err := g.Create(&editor).Error; err != nil {
		t.Fatalf("create editor: %v", err)
	}

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	adminToken, err := store.Create(1)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}
	editorToken, err := store.Create(editor.ID)
	if err != nil {
		t.Fatalf("Create editor session: %v", err)
	}

	r := gin.New()
	r.Use(auth.LoadUser(store, g))
	r.GET("/admin-only", auth.RequireRole("central_admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	tests := []struct {
		name   string
		cookie string
		want   int
	}{
		{"no cookie", "", http.StatusUnauthorized},
		{"editor token", editorToken, http.StatusForbidden},
		{"admin token", adminToken, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin-only", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "session", Value: tt.cookie})
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestLoadUserRejectsInactiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := newDB(t)

	inactive := false
	blocked := model.User{
		FullName:     "Blocked",
		Email:        "blocked@x",
		PasswordHash: "hash",
		Role:         "central_admin",
		Active:       &inactive,
	}
	if err := g.Create(&blocked).Error; err != nil {
		t.Fatalf("create blocked user: %v", err)
	}
	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	blockedToken, err := store.Create(blocked.ID)
	if err != nil {
		t.Fatalf("Create blocked session: %v", err)
	}

	r := gin.New()
	r.Use(auth.LoadUser(store, g))
	r.GET("/protected", auth.RequireAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// A model.User with Active == nil is not covered here: the users.active
	// column is NOT NULL with a GORM default of true, so GORM fills in true
	// on create and the DB itself rejects a later UPDATE to NULL. LoadUser's
	// nil check is defensive but not reachable through the database.
	tests := []struct {
		name  string
		token string
	}{
		{"active = false", blockedToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/protected", nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: tt.token})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestCanWriteDistrict(t *testing.T) {
	districtID := uint(1)
	other := uint(2)
	admin := &auth.CurrentUser{ID: 1, Role: "central_admin"}
	editor := &auth.CurrentUser{ID: 2, Role: "district_editor", DistrictID: &districtID}
	editorNoDistrict := &auth.CurrentUser{ID: 3, Role: "district_editor", DistrictID: nil}

	if !auth.CanWriteDistrict(admin, districtID) {
		t.Fatal("admin should write any district")
	}
	if !auth.CanWriteDistrict(admin, other) {
		t.Fatal("admin should write any district")
	}
	if !auth.CanWriteDistrict(editor, districtID) {
		t.Fatal("editor should write own district")
	}
	if auth.CanWriteDistrict(editor, other) {
		t.Fatal("editor should not write another district")
	}
	if auth.CanWriteDistrict(editorNoDistrict, districtID) {
		t.Fatal("editor with nil district should not write any district")
	}
}
