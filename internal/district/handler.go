package district

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

// districtRequest is the JSON body for POST/PUT /api/districts.
type districtRequest struct {
	Name     string `json:"name"`
	Province string `json:"province"`
}

// RegisterRoutes wires the district CRUD endpoints onto r. The caller must
// wrap r with auth.LoadUser first.
func RegisterRoutes(r gin.IRouter, repo *Repo) {
	r.GET("/api/districts", auth.RequireAuth(), listHandler(repo))
	r.POST("/api/districts", auth.RequireRole("central_admin"), createHandler(repo))
	r.PUT("/api/districts/:id", auth.RequireRole("central_admin"), updateHandler(repo))
	r.DELETE("/api/districts/:id", auth.RequireRole("central_admin"), deleteHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		districts, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list districts")
			return
		}
		httpx.OK(c, http.StatusOK, districts)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req districtRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "name and province are required")
			return
		}
		d := model.District{Name: req.Name, Province: req.Province}
		if err := repo.Create(&d); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create district")
			return
		}
		httpx.OK(c, http.StatusCreated, d)
	}
}

func updateHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var req districtRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "name and province are required")
			return
		}
		d := model.District{ID: id, Name: req.Name, Province: req.Province}
		if err := repo.Update(&d); err != nil {
			writeRepoError(c, err, "could not update district")
			return
		}
		httpx.OK(c, http.StatusOK, d)
	}
}

func deleteHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := repo.Delete(id); err != nil {
			writeRepoError(c, err, "could not delete district")
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
		httpx.Err(c, http.StatusNotFound, "not_found", "district not found")
		return
	}
	httpx.Err(c, http.StatusInternalServerError, "internal_error", msg)
}
