// Package webui serves the embedded Next.js single-page app.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/httpx"
)

//go:embed all:dist
var distFS embed.FS

// Register mounts the embedded static assets and installs a NoRoute
// handler that serves index.html for unknown client-side routes (SPA
// fallback), while returning real 404s for missing API calls and
// missing asset-looking paths.
func Register(r *gin.Engine) {
	root, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(root))

	r.NoRoute(func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if fileExists(root, path) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			httpx.Err(c, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		if looksLikeAsset(c.Request.URL.Path) {
			c.Status(http.StatusNotFound)
			return
		}
		serveIndex(c, root)
	})
}

// serveIndex writes index.html directly rather than through
// http.FileServer, which redirects any request whose path ends in
// "index.html" to "./" (FIX-13: SPA fallback must return 200, not 301).
func serveIndex(c *gin.Context, root fs.FS) {
	body, err := fs.ReadFile(root, "index.html")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
}

func looksLikeAsset(path string) bool {
	if strings.HasPrefix(path, "/_next/") {
		return true
	}
	seg := path[strings.LastIndex(path, "/")+1:]
	return strings.Contains(seg, ".")
}

func fileExists(root fs.FS, path string) bool {
	f, err := root.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	return err == nil && !info.IsDir()
}
