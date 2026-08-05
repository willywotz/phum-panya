package herb

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
)

// herbRequest is the JSON body for POST/PUT /api/herbs.
type herbRequest struct {
	ThaiName       string `json:"thai_name" binding:"required"`
	LocalName      string `json:"local_name"`
	ScientificName string `json:"scientific_name"`
	Photo          string `json:"photo"`
	PartUsed       string `json:"part_used"`
	Properties     string `json:"properties"`
}

// reconcileRequest is the JSON body for POST /api/herbs/reconcile.
type reconcileRequest struct {
	PendingName string `json:"pending_name" binding:"required"`
	HerbID      uint   `json:"herb_id" binding:"required"`
}

// RegisterRoutes wires the herb catalog, pending-herb, and storage-usage
// endpoints onto r. The caller must wrap r with auth.LoadUser first. A
// district editor may add herbs and edit ones its own district created;
// merging/aliasing and every other route require the central_admin role.
func RegisterRoutes(r gin.IRouter, repo *Repo, mediaStore *media.Store) {
	admin := auth.RequireRole("central_admin")
	authed := auth.RequireAuth()
	r.GET("/api/herbs", admin, listHandler(repo))
	r.GET("/api/herbs/near-duplicates", authed, nearDuplicatesHandler(repo))
	r.POST("/api/herbs", authed, createHandler(repo))
	r.PUT("/api/herbs/:id", authed, updateHandler(repo))
	r.POST("/api/herbs/:id/merge/:canonicalId", admin, mergeHandler(repo))
	r.DELETE("/api/herbs/:id", admin, deleteHandler(repo))
	r.GET("/api/herbs/pending", admin, pendingHandler(repo))
	r.POST("/api/herbs/reconcile", admin, reconcileHandler(repo))
	r.GET("/api/storage", admin, storageHandler(mediaStore))
}

// districtOf returns the editor's district for provenance/ownership, or nil for an admin.
func districtOf(c *gin.Context) *uint {
	user, ok := auth.UserFrom(c)
	if !ok || user.Role != model.RoleDistrictEditor {
		return nil
	}
	return user.DistrictID
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		herbs, err := repo.List()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list herbs")
			return
		}
		httpx.OK(c, http.StatusOK, herbs)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req herbRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid herb body")
			return
		}
		h := model.Herb{
			ThaiName: req.ThaiName, LocalName: req.LocalName, ScientificName: req.ScientificName,
			Photo: req.Photo, PartUsed: req.PartUsed, Properties: req.Properties,
		}
		if err := repo.Create(&h, districtOf(c)); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create herb")
			return
		}
		httpx.OK(c, http.StatusCreated, h)
	}
}

func updateHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var req herbRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid herb body")
			return
		}
		h := model.Herb{
			ID: id, ThaiName: req.ThaiName, LocalName: req.LocalName, ScientificName: req.ScientificName,
			Photo: req.Photo, PartUsed: req.PartUsed, Properties: req.Properties,
		}
		if err := repo.Update(&h, districtOf(c)); err != nil {
			if errors.Is(err, ErrNotOwner) {
				httpx.Err(c, http.StatusForbidden, "forbidden", "you may edit only herbs your district created")
				return
			}
			writeRepoError(c, err, "could not update herb")
			return
		}
		httpx.OK(c, http.StatusOK, h)
	}
}

func mergeHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		aliasID, ok := parseID(c)
		if !ok {
			return
		}
		canonical, err := strconv.ParseUint(c.Param("canonicalId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid canonical id")
			return
		}
		n, err := repo.Merge(aliasID, uint(canonical))
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not merge herbs")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"rePointed": n})
	}
}

func nearDuplicatesHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		dups, err := repo.NearDuplicates(c.Query("thaiName"))
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not check near duplicates")
			return
		}
		httpx.OK(c, http.StatusOK, dups)
	}
}

func deleteHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := repo.Delete(id); err != nil {
			writeRepoError(c, err, "could not delete herb")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func pendingHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		names, err := repo.PendingNames()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list pending herb names")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"pending_names": names})
	}
}

func reconcileHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req reconcileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "pending_name and herb_id are required")
			return
		}
		count, err := repo.Reconcile(req.PendingName, req.HerbID)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not reconcile pending herb")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"reconciled": count})
	}
}

func storageHandler(mediaStore *media.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		used, err := mediaStore.UsageBytes()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not compute storage usage")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"used_bytes": used})
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
		httpx.Err(c, http.StatusNotFound, "not_found", "herb not found")
		return
	}
	httpx.Err(c, http.StatusInternalServerError, "internal_error", msg)
}
