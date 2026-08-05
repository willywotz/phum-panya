package user

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// userRequest is the JSON body for PUT /api/users/:id. Role must match
// model.RoleCentralAdmin / model.RoleDistrictEditor: a struct tag is a
// literal string and cannot reference those constants directly, so keep the
// oneof list in sync with them by hand.
type userRequest struct {
	FullName   string `json:"full_name" binding:"required"`
	Email      string `json:"email" binding:"required"`
	Role       string `json:"role" binding:"required,oneof=central_admin district_editor"`
	DistrictID *uint  `json:"district_id"`
}

// createUserRequest is the JSON body for POST /api/users: it adds the
// password userRequest omits, since only account creation sets one (a
// PUT edit never carries a password; see setPasswordHandler for resets).
type createUserRequest struct {
	userRequest
	Password string `json:"password" binding:"required"`
}

// passwordRequest is the JSON body for POST /api/users/:id/password.
type passwordRequest struct {
	Password string `json:"password" binding:"required"`
}

// activeRequest is the JSON body for POST /api/users/:id/active.
type activeRequest struct {
	Active bool `json:"active"`
}

// RegisterRoutes wires the user CRUD endpoints onto r. Every route requires
// the central_admin role. The caller must wrap r with auth.LoadUser first.
func RegisterRoutes(r gin.IRouter, repo *Repo) {
	admin := auth.RequireRole("central_admin")
	r.GET("/api/users", admin, listHandler(repo))
	r.POST("/api/users", admin, createHandler(repo))
	r.PUT("/api/users/:id", admin, updateHandler(repo))
	r.POST("/api/users/:id/password", admin, setPasswordHandler(repo))
	r.POST("/api/users/:id/active", admin, setActiveHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list users")
			return
		}
		httpx.OK(c, http.StatusOK, users)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "full_name, email, password, and a valid role are required")
			return
		}
		if req.Role == "district_editor" && req.DistrictID == nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "district_id is required for district_editor")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not hash password")
			return
		}
		active := true
		u := model.User{
			FullName:     req.FullName,
			Email:        req.Email,
			PasswordHash: hash,
			Role:         req.Role,
			DistrictID:   req.DistrictID,
			Active:       &active,
		}
		if err := repo.Create(&u); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create user")
			return
		}
		httpx.OK(c, http.StatusCreated, u)
	}
}

func updateHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var req userRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "full_name, email, and a valid role are required")
			return
		}
		if req.Role == "district_editor" && req.DistrictID == nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "district_id is required for district_editor")
			return
		}
		u := model.User{
			ID:         id,
			FullName:   req.FullName,
			Email:      req.Email,
			Role:       req.Role,
			DistrictID: req.DistrictID,
		}
		if err := repo.Update(&u); err != nil {
			writeRepoError(c, err, "could not update user")
			return
		}
		httpx.OK(c, http.StatusOK, u)
	}
}

func setPasswordHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var req passwordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "password is required")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not hash password")
			return
		}
		if err := repo.SetPassword(id, hash); err != nil {
			writeRepoError(c, err, "could not set password")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func setActiveHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var req activeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "active is required")
			return
		}
		if err := repo.SetActive(id, req.Active); err != nil {
			writeRepoError(c, err, "could not set active")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// parseID parses the :id path parameter, writing a 400 response and
// returning ok=false on failure.
func parseID(c *gin.Context) (id uint, ok bool) {
	parsed, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return uint(parsed), true
}

// writeRepoError writes a 404 for a not-found repo error, or a 500 with msg
// for any other error.
func writeRepoError(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Err(c, http.StatusNotFound, "not_found", "user not found")
		return
	}
	httpx.Err(c, http.StatusInternalServerError, "internal_error", msg)
}
