package webui_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/webui"
)

func TestSPAFallbackAndApi404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	webui.Register(r)
	cases := []struct {
		path string
		want int
	}{
		{"/doctors/42", 200},
		{"/api/nope", 404},
		{"/_next/missing.js", 404},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s: %d, want %d", c.path, rec.Code, c.want)
		}
	}
}

func TestIndexHTMLDirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	webui.Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/index.html", nil))
	if rec.Code != 200 {
		t.Fatalf("/index.html: %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="app"`) {
		t.Errorf("/index.html body missing id=%q app marker: %s", "app", rec.Body.String())
	}
}
