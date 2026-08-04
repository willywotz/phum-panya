package review

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// RegisterRoutes wires the central-admin review queue endpoints.
func RegisterRoutes(r gin.IRouter, repo *Repo) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.GET("/api/review/queue", admin, queueHandler(repo))
	r.POST("/api/review/entry/:entityType/:entityId/approve", admin, approveHandler(repo))
	r.POST("/api/review/entry/:entityType/:entityId/reject", admin, rejectHandler(repo))
	r.POST("/api/review/doctor/:doctorId/approve-all", admin, approveTreeHandler(repo))
}

func queueHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := repo.Queue(nil)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "queue_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, items)
	}
}

func approveHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid entity id")
			return
		}
		if err := repo.Approve(c.Param("entityType"), uint(id), user.ID); err != nil {
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "approved"})
	}
}

func rejectHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Reason == "" {
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

func approveTreeHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		id, err := strconv.ParseUint(c.Param("doctorId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid doctor id")
			return
		}
		n, err := repo.ApproveDoctorTree(uint(id), user.ID)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "approve_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"approved": n})
	}
}
