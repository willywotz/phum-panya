package doctor

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
)

// doctorRequest is the JSON body for POST/PUT /api/doctors.
type doctorRequest struct {
	Code            string     `json:"code"`
	Photo           string     `json:"photo"`
	FullName        string     `json:"full_name"`
	KnownAs         string     `json:"known_as"`
	Gender          string     `json:"gender"`
	BirthYear       int        `json:"birth_year"`
	DistrictID      uint       `json:"district_id"`
	Address         string     `json:"address"`
	Phone           string     `json:"phone"`
	Specialty       []string   `json:"specialty"`
	YearsExperience int        `json:"years_experience"`
	Lineage         string     `json:"lineage"`
	ConsentObtained bool       `json:"consent_obtained"`
	ConsentDate     *time.Time `json:"consent_date"`
	Status          string     `json:"status"`
	FirstYear       int        `json:"first_year"`
}

// toModel builds a model.Doctor from req, joining Specialty into a
// comma-separated string.
func (req doctorRequest) toModel(id uint) model.Doctor {
	return model.Doctor{
		ID:              id,
		Code:            req.Code,
		Photo:           req.Photo,
		FullName:        req.FullName,
		KnownAs:         req.KnownAs,
		Gender:          req.Gender,
		BirthYear:       req.BirthYear,
		DistrictID:      req.DistrictID,
		Address:         req.Address,
		Phone:           req.Phone,
		Specialty:       strings.Join(req.Specialty, ","),
		YearsExperience: req.YearsExperience,
		Lineage:         req.Lineage,
		ConsentObtained: req.ConsentObtained,
		ConsentDate:     req.ConsentDate,
		Status:          req.Status,
		FirstYear:       req.FirstYear,
	}
}

// RegisterRoutes wires the doctor CRUD and photo-upload endpoints onto r.
// The caller must wrap r with auth.LoadUser first. Writes are restricted to
// the doctor's own district via auth.CanWriteDistrict.
func RegisterRoutes(r gin.IRouter, repo *Repo, mediaStore *media.Store) {
	requireAuth := auth.RequireAuth()
	r.GET("/api/doctors", requireAuth, listHandler(repo))
	r.POST("/api/doctors", requireAuth, createHandler(repo))
	r.PUT("/api/doctors/:id", requireAuth, updateHandler(repo))
	r.DELETE("/api/doctors/:id", requireAuth, deleteHandler(repo))
	r.POST("/api/doctors/:id/photo", requireAuth, photoHandler(repo, mediaStore))
	r.POST("/api/doctors/:id/unpublish", requireAuth, unpublishHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		districtID, ok := parseDistrictQuery(c)
		if !ok {
			return
		}
		doctors, err := repo.ListByDistrict(districtID)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list doctors")
			return
		}
		httpx.OK(c, http.StatusOK, doctors)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req doctorRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid doctor body")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, req.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		d := req.toModel(0)
		if err := repo.Create(&d, user.ID, immediate); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create doctor")
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
		existing, err := repo.Get(id)
		if err != nil {
			writeRepoError(c, err, "could not update doctor")
			return
		}
		var req doctorRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid doctor body")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, existing.DistrictID) || !auth.CanWriteDistrict(user, req.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		d := req.toModel(id)
		if err := repo.Update(&d, user.ID, immediate); err != nil {
			writeRepoError(c, err, "could not update doctor")
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
		existing, err := repo.Get(id)
		if err != nil {
			writeRepoError(c, err, "could not delete doctor")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, existing.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		if err := repo.Delete(id, user.ID, immediate); err != nil {
			writeRepoError(c, err, "could not delete doctor")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// unpublishHandler clears the doctor's consent_obtained flag, hiding it from
// the public view without deleting its rows.
func unpublishHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		existing, err := repo.Get(id)
		if err != nil {
			writeRepoError(c, err, "could not unpublish doctor")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, existing.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		if err := repo.Unpublish(id); err != nil {
			writeRepoError(c, err, "could not unpublish doctor")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func photoHandler(repo *Repo, mediaStore *media.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		existing, err := repo.Get(id)
		if err != nil {
			writeRepoError(c, err, "could not update doctor photo")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, existing.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		fh, err := c.FormFile("photo")
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "photo file is required")
			return
		}
		path, err := mediaStore.SaveMultipart(fh)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "could not process photo")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		if err := repo.SetPhoto(id, user.ID, path, immediate); err != nil {
			writeRepoError(c, err, "could not update doctor photo")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"photo": path})
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

// parseDistrictQuery parses the required district_id query parameter,
// writing a 400 response and returning ok=false on failure.
func parseDistrictQuery(c *gin.Context) (districtID uint, ok bool) {
	parsed, err := strconv.ParseUint(c.Query("district_id"), 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "district_id query parameter is required")
		return 0, false
	}
	return uint(parsed), true
}

// writeRepoError writes a 404 for a not-found repo error, or a 500 with msg
// for any other error.
func writeRepoError(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Err(c, http.StatusNotFound, "not_found", "doctor not found")
		return
	}
	httpx.Err(c, http.StatusInternalServerError, "internal_error", msg)
}
