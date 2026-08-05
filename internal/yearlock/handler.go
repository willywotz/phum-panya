package yearlock

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// repository is the port RegisterRoutes depends on; *Repo is the GORM
// adapter that implements it.
type repository interface {
	IsLocked(dataYear int) (bool, error)
	Lock(dataYear int, actorID uint) error
	Unlock(dataYear int) error
	List() ([]model.YearLock, error)
}

func RegisterRoutes(r gin.IRouter, repo repository) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/year-locks", admin, listHandler(repo))
	r.POST("/api/year-locks", admin, lockHandler(repo))
	r.DELETE("/api/year-locks/:dataYear", admin, unlockHandler(repo))
}

func listHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		locks, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "list_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, locks)
	}
}

func lockHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			DataYear int `json:"dataYear" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_year", "dataYear is required")
			return
		}
		if err := repo.Lock(body.DataYear, user.ID); err != nil {
			if errors.Is(err, ErrPendingInYear) {
				httpx.Err(c, http.StatusConflict, "pending_in_year", "clear the pending queue for this year first")
				return
			}
			httpx.Err(c, http.StatusInternalServerError, "lock_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusCreated, gin.H{"dataYear": body.DataYear})
	}
}

func unlockHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		year, err := strconv.Atoi(c.Param("dataYear"))
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_year", "invalid data year")
			return
		}
		if err := repo.Unlock(year); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "unlock_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"dataYear": year})
	}
}
