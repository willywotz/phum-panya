package webui_test

import (
	"net/http/httptest"
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
