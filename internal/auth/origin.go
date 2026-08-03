package auth

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/httpx"
)

// SameOrigin returns a middleware that rejects unsafe requests (POST, PUT,
// PATCH, DELETE) whose Origin (or, if absent, Referer) header does not
// name allowedHost. It is the second line of CSRF defense, behind
// SameSite=Strict cookies; no CSRF token is used. Safe methods always
// pass. If allowedHost is empty (dev mode), the middleware is a no-op.
func SameOrigin(allowedHost string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if allowedHost == "" || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if requestHost(c.Request) == allowedHost {
			c.Next()
			return
		}
		httpx.Err(c, http.StatusForbidden, "forbidden_origin", "request origin does not match this site")
		c.Abort()
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// requestHost returns the host from the Origin header, falling back to
// Referer if Origin is absent. It returns "" if neither header is set or
// parseable.
func requestHost(r *http.Request) string {
	raw := r.Header.Get("Origin")
	if raw == "" {
		raw = r.Header.Get("Referer")
	}
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
