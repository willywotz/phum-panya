package review

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
	"phum-panya/internal/yearlock"
)

// repository is the port RegisterRoutes depends on; *Repo is the GORM
// adapter that implements it.
type repository interface {
	Queue(districtID *uint) ([]Item, error)
	Approve(entityType string, entityID, actorID uint) error
	Reject(entityType string, entityID, actorID uint, reason string) error
	ApproveDoctorTree(doctorID, actorID uint) (int, error)
	Detail(entityType string, id uint) (Detail, error)
}

// RegisterRoutes wires the central-admin review queue endpoints.
func RegisterRoutes(r gin.IRouter, repo repository) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/review/queue", admin, queueHandler(repo))
	r.GET("/api/review/entry/:entityType/:entityId", admin, detailHandler(repo))
	r.POST("/api/review/entry/:entityType/:entityId/approve", admin, approveHandler(repo))
	r.POST("/api/review/entry/:entityType/:entityId/reject", admin, rejectHandler(repo))
	r.POST("/api/review/doctor/:doctorId/approve-all", admin, approveTreeHandler(repo))
}

func queueHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := repo.Queue(nil)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "queue_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, items)
	}
}

func detailHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		detail, err := repo.Detail(c.Param("entityType"), uint(id))
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "detail_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, detail)
	}
}

func approveHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		if err := repo.Approve(c.Param("entityType"), uint(id), user.ID); err != nil {
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "the target data year is locked")
				return
			}
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "approved"})
	}
}

func rejectHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			Reason string `json:"reason" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Err(c, http.StatusBadRequest, "reason_required", "a rejection reason is required")
			return
		}
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		if err := repo.Reject(c.Param("entityType"), uint(id), user.ID, body.Reason); err != nil {
			httpx.Err(c, http.StatusBadRequest, "reject_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "rejected"})
	}
}

func approveTreeHandler(repo repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("doctorId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid doctor id")
			return
		}
		n, err := repo.ApproveDoctorTree(uint(id), user.ID)
		if err != nil {
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "the target data year is locked")
				return
			}
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"approved": n})
	}
}
