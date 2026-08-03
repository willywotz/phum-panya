package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
)

func TestSameOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(auth.SameOrigin("app.test"))
	r.Any("/write", func(c *gin.Context) { c.Status(http.StatusOK) })

	tests := []struct {
		name    string
		method  string
		origin  string
		referer string
		want    int
	}{
		{"cross-origin POST", http.MethodPost, "https://evil.test", "", http.StatusForbidden},
		{"same-origin POST", http.MethodPost, "https://app.test", "", http.StatusOK},
		{"GET any origin", http.MethodGet, "https://evil.test", "", http.StatusOK},
		{"POST no origin, matching referer", http.MethodPost, "", "https://app.test/page", http.StatusOK},
		{"POST no origin no referer", http.MethodPost, "", "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/write", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestSameOriginDevModeNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(auth.SameOrigin(""))
	r.Any("/write", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
