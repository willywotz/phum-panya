package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

// A static export writes each app-router route to its own "<route>.html"
// shell (Next.js's clean-URL convention). The SPA fallback must serve that
// specific shell for a matching client route, not silently fall back to
// index.html, or hydration renders the wrong page.
func TestNoRouteServesMatchingHTMLShell(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := fstest.MapFS{
		"index.html": {Data: []byte(`<html id="home"></html>`)},
		"login.html": {Data: []byte(`<html id="login"></html>`)},
	}
	r := gin.New()
	register(r, root)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/login", nil))

	if rec.Code != 200 {
		t.Fatalf("/login: %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="login"`) {
		t.Errorf("/login served wrong shell: %s", rec.Body.String())
	}
}
