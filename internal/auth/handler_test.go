package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// newLoginRouter seeds the admin's password hash, wires the auth routes, and
// returns the engine plus the seeded admin email.
func newLoginRouter(t *testing.T, max int) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := newDB(t)

	hash, err := auth.HashPassword("pw12345")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := g.Model(&model.User{}).Where("id = ?", 1).
		Update("password_hash", hash).Error; err != nil {
		t.Fatalf("update password hash: %v", err)
	}

	store := auth.NewSessionStore(g, clock.Real{}, time.Hour)
	th := auth.NewThrottle(clock.Real{}, max, time.Minute)

	r := gin.New()
	auth.RegisterRoutes(r, g, store, th, false)

	return r, "admin@x"
}

func login(r *gin.Engine, email, password string) *httptest.ResponseRecorder {
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLoginSuccess(t *testing.T) {
	r, email := newLoginRouter(t, 5)

	rec := login(r, email, "pw12345")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, ck := range cookies {
		if ck.Name == "session" && ck.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Set-Cookie missing session token: %v", rec.Header().Get("Set-Cookie"))
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["role"] != "central_admin" {
		t.Fatalf("role = %v, want central_admin", body["role"])
	}
}

func TestLoginWrongPassword(t *testing.T) {
	r, email := newLoginRouter(t, 5)

	rec := login(r, email, "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	r, _ := newLoginRouter(t, 5)

	rec := login(r, "nobody@x", "pw12345")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginThrottled(t *testing.T) {
	r, email := newLoginRouter(t, 2)

	for i := 0; i < 2; i++ {
		rec := login(r, email, "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, rec.Code)
		}
	}

	rec := login(r, email, "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}

	// Even the correct password is now blocked.
	rec = login(r, email, "pw12345")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	r, email := newLoginRouter(t, 5)

	loginRec := login(r, email, "pw12345")
	var sessionCookie *http.Cookie
	for _, ck := range loginRec.Result().Cookies() {
		if ck.Name == "session" {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie from login")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", rec.Code)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	r.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", meRec.Code)
	}
}

func TestMeRequiresSession(t *testing.T) {
	r, _ := newLoginRouter(t, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
