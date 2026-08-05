package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// loginRequest is the JSON body for POST /api/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// dummyHash is a precomputed bcrypt hash (cost 12) of an unused value. It is
// compared against on every failed lookup so that a login with an unknown or
// inactive email costs the same bcrypt work as a login with a known email and
// wrong password, preventing account enumeration via response timing.
const dummyHash = "$2a$12$NvMgpYk902bQzaPhuRHRNukvkV2aF7/XOwd5wvua3z5dkgbgdi4Lq"

// RegisterRoutes wires the login, logout, and current-user endpoints onto r.
func RegisterRoutes(r gin.IRouter, g *gorm.DB, store *SessionStore, th *Throttle, secure bool) {
	r.POST("/api/login", loginHandler(g, store, th, secure))
	r.POST("/api/logout", logoutHandler(store))
	r.GET("/api/current-user", LoadUser(store, g), currentUserHandler)
}

func loginHandler(g *gorm.DB, store *SessionStore, th *Throttle, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "email and password are required")
			return
		}

		key := req.Email + "|" + c.ClientIP()
		if !th.Allowed(key) {
			httpx.Err(c, http.StatusTooManyRequests, "too_many_attempts", "too many login attempts, try again later")
			return
		}

		var user model.User
		err := g.Where("email = ? AND active = ?", req.Email, true).First(&user).Error
		hash := dummyHash
		if err == nil {
			hash = user.PasswordHash
		}
		// Evaluate CheckPassword unconditionally: err != nil must not
		// short-circuit the bcrypt call, or an unknown/inactive email would
		// return faster than a known email with a wrong password.
		passwordOK := CheckPassword(hash, req.Password)
		if err != nil || !passwordOK {
			th.Fail(key)
			httpx.Err(c, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
			return
		}

		th.Reset(key)
		raw, err := store.Create(user.ID)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create session")
			return
		}
		SetSessionCookie(c, raw, secure)
		httpx.OK(c, http.StatusOK, gin.H{
			"id":          user.ID,
			"full_name":   user.FullName,
			"role":        user.Role,
			"district_id": user.DistrictID,
		})
	}
}

func logoutHandler(store *SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw, err := c.Cookie(sessionCookieName); err == nil && raw != "" {
			_ = store.Delete(raw)
		}
		ClearSessionCookie(c)
		c.Status(http.StatusNoContent)
	}
}

func currentUserHandler(c *gin.Context) {
	user, ok := UserFrom(c)
	if !ok {
		httpx.Err(c, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{
		"id":          user.ID,
		"full_name":   user.FullName,
		"role":        user.Role,
		"district_id": user.DistrictID,
	})
}
