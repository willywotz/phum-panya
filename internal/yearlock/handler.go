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

func RegisterRoutes(r gin.IRouter, repo *Repo) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/year-locks", admin, listHandler(repo))
	r.POST("/api/year-locks", admin, lockHandler(repo))
	r.DELETE("/api/year-locks/:dataYear", admin, unlockHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		locks, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "list_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, locks)
	}
}

func lockHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			DataYear int `json:"dataYear"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.DataYear == 0 {
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

func unlockHandler(repo *Repo) gin.HandlerFunc {
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
