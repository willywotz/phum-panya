package caserec

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
	"phum-panya/internal/yearlock"
)

// validResults holds the result values the "result" DB check accepts.
// Validating here keeps an invalid value a clean 400, not a DB-CHECK 500.
var validResults = map[string]bool{
	"cured":     true,
	"better":    true,
	"no_change": true,
}

// caseRequest is the JSON body for POST/PUT /api/cases.
type caseRequest struct {
	RecipeID        uint   `json:"recipe_id"`
	PatientGender   string `json:"patient_gender"`
	PatientAgeRange string `json:"patient_age_range"`
	Condition       string `json:"condition"`
	Treatment       string `json:"treatment"`
	Result          string `json:"result"`
	Duration        string `json:"duration"`
	DataYear        int    `json:"data_year"`
}

// toModel builds a model.Case from req.
func (req caseRequest) toModel(id uint) model.Case {
	return model.Case{
		ID:              id,
		RecipeID:        req.RecipeID,
		PatientGender:   req.PatientGender,
		PatientAgeRange: req.PatientAgeRange,
		Condition:       req.Condition,
		Treatment:       req.Treatment,
		Result:          req.Result,
		Duration:        req.Duration,
		DataYear:        req.DataYear,
	}
}

// RegisterRoutes wires the case CRUD and photo-upload endpoints onto r. The
// caller must wrap r with auth.LoadUser first. Writes are restricted to the
// case's recipe's doctor's own district via auth.CanWriteDistrict.
func RegisterRoutes(r gin.IRouter, repo *Repo, mediaStore *media.Store) {
	requireAuth := auth.RequireAuth()
	r.GET("/api/cases", requireAuth, listHandler(repo))
	r.POST("/api/cases", requireAuth, createHandler(repo))
	r.PUT("/api/cases/:id", requireAuth, updateHandler(repo))
	r.DELETE("/api/cases/:id", requireAuth, deleteHandler(repo))
	r.POST("/api/cases/:id/photo", requireAuth, photoHandler(repo, mediaStore))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		recipeID, ok := parseRecipeQuery(c)
		if !ok {
			return
		}
		cases, err := repo.ListByRecipe(recipeID)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list cases")
			return
		}
		httpx.OK(c, http.StatusOK, cases)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req caseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid case body")
			return
		}
		if !validResults[req.Result] {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "result must be one of cured, better, no_change")
			return
		}
		districtID, err := repo.DistrictOf(req.RecipeID)
		if err != nil {
			writeRepoError(c, err, "recipe not found", "could not resolve recipe district")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, districtID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		cs := req.toModel(0)
		if err := repo.Create(&cs, user.ID, immediate); err != nil {
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "this data year is locked")
				return
			}
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create case")
			return
		}
		httpx.OK(c, http.StatusCreated, cs)
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
			writeRepoError(c, err, "case not found", "could not update case")
			return
		}
		var req caseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid case body")
			return
		}
		if !validResults[req.Result] {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "result must be one of cured, better, no_change")
			return
		}
		oldDistrict, err := repo.DistrictOf(existing.RecipeID)
		if err != nil {
			writeRepoError(c, err, "recipe not found", "could not resolve recipe district")
			return
		}
		newDistrict, err := repo.DistrictOf(req.RecipeID)
		if err != nil {
			writeRepoError(c, err, "recipe not found", "could not resolve recipe district")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, oldDistrict) || !auth.CanWriteDistrict(user, newDistrict) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		cs := req.toModel(id)
		if err := repo.Update(&cs, user.ID, immediate); err != nil {
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "this data year is locked")
				return
			}
			writeRepoError(c, err, "case not found", "could not update case")
			return
		}
		httpx.OK(c, http.StatusOK, cs)
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
			writeRepoError(c, err, "case not found", "could not delete case")
			return
		}
		districtID, err := repo.DistrictOf(existing.RecipeID)
		if err != nil {
			writeRepoError(c, err, "recipe not found", "could not resolve recipe district")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, districtID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		immediate := user.Role == model.RoleCentralAdmin
		if err := repo.Delete(id, user.ID, immediate); err != nil {
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "this data year is locked")
				return
			}
			writeRepoError(c, err, "case not found", "could not delete case")
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
			writeRepoError(c, err, "case not found", "could not update case photo")
			return
		}
		districtID, err := repo.DistrictOf(existing.RecipeID)
		if err != nil {
			writeRepoError(c, err, "recipe not found", "could not resolve recipe district")
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, districtID) {
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
			if errors.Is(err, yearlock.ErrYearLocked) {
				httpx.Err(c, http.StatusConflict, "year_locked", "this data year is locked")
				return
			}
			writeRepoError(c, err, "case not found", "could not update case photo")
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

// parseRecipeQuery parses the required recipe_id query parameter, writing a
// 400 response and returning ok=false on failure.
func parseRecipeQuery(c *gin.Context) (recipeID uint, ok bool) {
	parsed, err := strconv.ParseUint(c.Query("recipe_id"), 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "recipe_id query parameter is required")
		return 0, false
	}
	return uint(parsed), true
}

// writeRepoError writes a 404 with notFoundMsg for a not-found repo error,
// or a 500 with msg for any other error.
func writeRepoError(c *gin.Context, err error, notFoundMsg, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Err(c, http.StatusNotFound, "not_found", notFoundMsg)
		return
	}
	httpx.Err(c, http.StatusInternalServerError, "internal_error", msg)
}
