package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"phum-panya/internal/auth"
	"phum-panya/internal/bootstrap"
	"phum-panya/internal/clock"
	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/media"
	"phum-panya/internal/router"
)

// newTestServer builds a fresh temp-DB app (seeded with a central admin) and
// returns an httptest.Server serving the fully wired engine.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()

	g, err := db.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if _, err := bootstrap.EnsureAdmin(g, "admin@test", "adminpass1"); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}

	deps := router.Deps{
		Cfg:        config.Config{DevMode: true},
		DB:         g,
		Store:      auth.NewSessionStore(g, clock.Real{}, 24*time.Hour),
		Throttle:   auth.NewThrottle(clock.Real{}, 100, time.Minute),
		Media:      &media.LocalStore{Dir: filepath.Join(dir, "media")},
		Clk:        clock.Real{},
		Secure:     false,
		BackupDir:  filepath.Join(dir, "backup"),
		BackupKeep: 7,
		DBPath:     filepath.Join(dir, "app.db"),
		MediaDir:   filepath.Join(dir, "media"),
	}
	engine := router.NewEngine(deps)
	return httptest.NewServer(engine)
}

// newClient returns an http.Client with its own cookie jar, so login
// sessions persist across requests to srv.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{Jar: jar}
}

// doJSON sends method to srv.URL+path with body (nil allowed) and decodes
// the JSON response into out (nil allowed).
func doJSON(t *testing.T, client *http.Client, method, url, body string, out any) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s %s: %v", method, url, err)
		}
	}
	return resp
}

// TestUATFlow drives the SRS UAT steps 1-9 end to end against the fully
// wired engine: admin login, district + editor creation, editor login,
// doctor + recipe (with a pending herb) + reconcile + case creation, then
// confirms the doctor surfaces on the public API.
func TestUATFlow(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	admin := newClient(t)

	// 1. Log in as the admin.
	loginResp := doJSON(t, admin, http.MethodPost, srv.URL+"/api/login",
		`{"email":"admin@test","password":"adminpass1"}`, nil)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("admin login status = %d, want 200", loginResp.StatusCode)
	}

	// 2. Create a district.
	var district struct {
		ID uint `json:"ID"`
	}
	distResp := doJSON(t, admin, http.MethodPost, srv.URL+"/api/districts",
		`{"name":"Muang","province":"Test"}`, &district)
	if distResp.StatusCode != http.StatusCreated {
		t.Fatalf("create district status = %d, want 201", distResp.StatusCode)
	}
	if district.ID == 0 {
		t.Fatalf("district.ID = 0, want non-zero")
	}

	// 3. Create a district_editor user for that district.
	editorEmail := "editor@test"
	editorReq := `{"full_name":"Editor","email":"` + editorEmail + `","password":"editorpass1","role":"district_editor","district_id":` +
		strconv.Itoa(int(district.ID)) + `}`
	userResp := doJSON(t, admin, http.MethodPost, srv.URL+"/api/users", editorReq, nil)
	if userResp.StatusCode != http.StatusCreated {
		t.Fatalf("create user status = %d, want 201", userResp.StatusCode)
	}

	// 4. Log in as that editor.
	editor := newClient(t)
	editorLogin := doJSON(t, editor, http.MethodPost, srv.URL+"/api/login",
		`{"email":"`+editorEmail+`","password":"editorpass1"}`, nil)
	if editorLogin.StatusCode != http.StatusOK {
		t.Fatalf("editor login status = %d, want 200", editorLogin.StatusCode)
	}

	// GET /api/current-user returns the logged-in editor; /api/me is gone.
	var currentUser struct {
		Role string `json:"role"`
	}
	curResp := doJSON(t, editor, http.MethodGet, srv.URL+"/api/current-user", "", &currentUser)
	if curResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/current-user status = %d, want 200", curResp.StatusCode)
	}
	if currentUser.Role != "district_editor" {
		t.Fatalf("current-user role = %q, want district_editor", currentUser.Role)
	}
	meResp := doJSON(t, editor, http.MethodGet, srv.URL+"/api/me", "", nil)
	if meResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/me status = %d, want 404 (route renamed)", meResp.StatusCode)
	}

	// 5. Create a doctor in that district, already consented.
	var doctor struct {
		ID uint `json:"ID"`
	}
	doctorReq := `{"code":"D001","photo":"x.jpg","full_name":"Somchai","district_id":` + strconv.Itoa(int(district.ID)) +
		`,"specialty":["herbal"],"consent_obtained":true,"consent_date":"2024-01-01T00:00:00Z","status":"active","first_year":2000}`
	doctorResp := doJSON(t, editor, http.MethodPost, srv.URL+"/api/doctors", doctorReq, &doctor)
	if doctorResp.StatusCode != http.StatusCreated {
		t.Fatalf("create doctor status = %d, want 201", doctorResp.StatusCode)
	}

	// 5b. Admin approves the pending doctor (editor writes are queued in P2).
	approveReq := `{"code":"D001","photo":"x.jpg","full_name":"Somchai","district_id":` + strconv.Itoa(int(district.ID)) +
		`,"specialty":["herbal"],"consent_obtained":true,"consent_date":"2024-01-01T00:00:00Z","status":"active","first_year":2000}`
	approveResp := doJSON(t, admin, http.MethodPut, srv.URL+"/api/doctors/"+strconv.Itoa(int(doctor.ID)), approveReq, nil)
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("admin approve doctor status = %d, want 200", approveResp.StatusCode)
	}

	// 6. Create a recipe with an ingredient using a pending_herb_name.
	var recipe struct {
		ID uint `json:"ID"`
	}
	recipeReq := `{"code":"R001","name":"Yaa Klai","doctor_id":` + strconv.Itoa(int(doctor.ID)) +
		`,"indication":"fever","preparation":"boil","usage":"drink","data_year":2024,` +
		`"ingredients":[{"pending_herb_name":"Fai Daeng","amount":"1","unit":"handful"}]}`
	recipeResp := doJSON(t, editor, http.MethodPost, srv.URL+"/api/recipes", recipeReq, &recipe)
	if recipeResp.StatusCode != http.StatusCreated {
		t.Fatalf("create recipe status = %d, want 201", recipeResp.StatusCode)
	}

	// 7. As admin, reconcile the pending herb.
	var herb struct {
		ID uint `json:"ID"`
	}
	herbResp := doJSON(t, admin, http.MethodPost, srv.URL+"/api/herbs",
		`{"thai_name":"Fai Daeng","part_used":"leaf"}`, &herb)
	if herbResp.StatusCode != http.StatusCreated {
		t.Fatalf("create herb status = %d, want 201", herbResp.StatusCode)
	}
	reconcileResp := doJSON(t, admin, http.MethodPost, srv.URL+"/api/herbs/reconcile",
		`{"pending_name":"Fai Daeng","herb_id":`+strconv.Itoa(int(herb.ID))+`}`, nil)
	if reconcileResp.StatusCode != http.StatusOK {
		t.Fatalf("reconcile status = %d, want 200", reconcileResp.StatusCode)
	}

	// 8. Create a case (as the editor, who owns the recipe's district).
	caseReq := `{"recipe_id":` + strconv.Itoa(int(recipe.ID)) +
		`,"patient_gender":"female","patient_age_range":"30-40","condition":"fever","result":"cured","data_year":2024}`
	caseResp := doJSON(t, editor, http.MethodPost, srv.URL+"/api/cases", caseReq, nil)
	if caseResp.StatusCode != http.StatusCreated {
		t.Fatalf("create case status = %d, want 201", caseResp.StatusCode)
	}

	// 9. GET /api/public/doctors shows the consented doctor.
	anon := newClient(t)
	var publicDoctors []struct {
		FullName string `json:"full_name"`
	}
	publicResp := doJSON(t, anon, http.MethodGet, srv.URL+"/api/public/doctors", "", &publicDoctors)
	if publicResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/public/doctors status = %d, want 200", publicResp.StatusCode)
	}
	found := false
	for _, d := range publicDoctors {
		if d.FullName == "Somchai" {
			found = true
		}
	}
	if !found {
		t.Fatalf("public doctors = %+v, want to contain Somchai", publicDoctors)
	}

	// Unknown API route -> JSON 404; unknown frontend route -> SPA fallback 200.
	notFoundResp := doJSON(t, anon, http.MethodGet, srv.URL+"/api/whatever", "", nil)
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/whatever status = %d, want 404", notFoundResp.StatusCode)
	}
	spaResp := doJSON(t, anon, http.MethodGet, srv.URL+"/somefrontendroute", "", nil)
	if spaResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /somefrontendroute status = %d, want 200 (SPA fallback)", spaResp.StatusCode)
	}
}

func TestNewMediaStoreLocal(t *testing.T) {
	s, err := newMediaStore(context.Background(), config.Config{MediaDriver: "local", MediaDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newMediaStore: %v", err)
	}
	if _, ok := s.(*media.LocalStore); !ok {
		t.Fatalf("got %T, want *media.LocalStore", s)
	}
}

func TestNewLimiterMemory(t *testing.T) {
	l := newLimiter(nil, config.Config{ThrottleStore: "memory"}, clock.Real{})
	if _, ok := l.(*auth.Throttle); !ok {
		t.Fatalf("got %T, want *auth.Throttle", l)
	}
}
