package recipe

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

// ingredientRequest is one ingredient in the JSON body for POST/PUT
// /api/recipes.
type ingredientRequest struct {
	HerbID          *uint   `json:"herb_id"`
	PendingHerbName *string `json:"pending_herb_name"`
	Amount          string  `json:"amount"`
	Unit            string  `json:"unit"`
	Note            string  `json:"note"`
}

// toModel builds a model.Ingredient from req.
func (req ingredientRequest) toModel() model.Ingredient {
	return model.Ingredient{
		HerbID:          req.HerbID,
		PendingHerbName: req.PendingHerbName,
		Amount:          req.Amount,
		Unit:            req.Unit,
		Note:            req.Note,
	}
}

// valid reports whether exactly one of HerbID or PendingHerbName is set.
func (req ingredientRequest) valid() bool {
	herbSet := req.HerbID != nil
	pendingSet := req.PendingHerbName != nil && *req.PendingHerbName != ""
	return herbSet != pendingSet
}

// recipeRequest is the JSON body for POST/PUT /api/recipes.
type recipeRequest struct {
	Code        string              `json:"code"`
	Name        string              `json:"name"`
	DoctorID    uint                `json:"doctor_id"`
	Indication  string              `json:"indication"`
	Preparation string              `json:"preparation"`
	Usage       string              `json:"usage"`
	Caution     string              `json:"caution"`
	CareStage   string              `json:"care_stage"`
	DataYear    int                 `json:"data_year"`
	Ingredients []ingredientRequest `json:"ingredients"`
}

// toModel builds a model.Recipe from req.
func (req recipeRequest) toModel(id uint) model.Recipe {
	return model.Recipe{
		ID:          id,
		Code:        req.Code,
		Name:        req.Name,
		DoctorID:    req.DoctorID,
		Indication:  req.Indication,
		Preparation: req.Preparation,
		Usage:       req.Usage,
		Caution:     req.Caution,
		CareStage:   req.CareStage,
		DataYear:    req.DataYear,
	}
}

// toIngredients builds a model.Ingredient slice from req.Ingredients.
func (req recipeRequest) toIngredients() []model.Ingredient {
	ings := make([]model.Ingredient, len(req.Ingredients))
	for i, ing := range req.Ingredients {
		ings[i] = ing.toModel()
	}
	return ings
}

// invalidIngredient returns the index of the first ingredient that does not
// set exactly one of herb_id / pending_herb_name, or -1 if all are valid.
func (req recipeRequest) invalidIngredient() int {
	for i, ing := range req.Ingredients {
		if !ing.valid() {
			return i
		}
	}
	return -1
}

// recipeResponse composes a recipe with its ingredients.
type recipeResponse struct {
	model.Recipe
	Ingredients []model.Ingredient `json:"ingredients"`
}

// RegisterRoutes wires the recipe CRUD and doctor-resolution endpoints onto
// r. The caller must wrap r with auth.LoadUser first. Writes are restricted
// to the recipe's doctor's own district via auth.CanWriteDistrict.
func RegisterRoutes(r gin.IRouter, repo *Repo) {
	requireAuth := auth.RequireAuth()
	r.GET("/api/recipes", requireAuth, listHandler(repo))
	r.POST("/api/recipes", requireAuth, createHandler(repo))
	r.PUT("/api/recipes/:id", requireAuth, updateHandler(repo))
	r.DELETE("/api/recipes/:id", requireAuth, deleteHandler(repo))
	r.GET("/api/recipes/resolve-doctor", requireAuth, resolveDoctorHandler(repo))
}

func listHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		doctorID, ok := parseDoctorQuery(c)
		if !ok {
			return
		}
		recipes, err := repo.ListByDoctor(doctorID)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list recipes")
			return
		}
		out := make([]recipeResponse, len(recipes))
		for i, rec := range recipes {
			ings, err := repo.GetIngredients(rec.ID)
			if err != nil {
				httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list ingredients")
				return
			}
			out[i] = recipeResponse{Recipe: rec, Ingredients: ings}
		}
		httpx.OK(c, http.StatusOK, out)
	}
}

func createHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req recipeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid recipe body")
			return
		}
		if i := req.invalidIngredient(); i >= 0 {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "ingredient must set exactly one of herb_id or pending_herb_name")
			return
		}
		doctor, err := doctorOf(c, repo, req.DoctorID)
		if err != nil {
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, doctor.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		rec := req.toModel(0)
		ings := req.toIngredients()
		if err := repo.Create(&rec, ings, user.ID); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not create recipe")
			return
		}
		httpx.OK(c, http.StatusCreated, recipeResponse{Recipe: rec, Ingredients: ings})
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
			writeRepoError(c, err, "recipe not found", "could not update recipe")
			return
		}
		var req recipeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid recipe body")
			return
		}
		if i := req.invalidIngredient(); i >= 0 {
			httpx.Err(c, http.StatusBadRequest, "invalid_request", "ingredient must set exactly one of herb_id or pending_herb_name")
			return
		}
		existingDoctor, err := doctorOf(c, repo, existing.DoctorID)
		if err != nil {
			return
		}
		newDoctor, err := doctorOf(c, repo, req.DoctorID)
		if err != nil {
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, existingDoctor.DistrictID) || !auth.CanWriteDistrict(user, newDoctor.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		rec := req.toModel(id)
		ings := req.toIngredients()
		if err := repo.Update(&rec, ings, user.ID); err != nil {
			writeRepoError(c, err, "recipe not found", "could not update recipe")
			return
		}
		httpx.OK(c, http.StatusOK, recipeResponse{Recipe: rec, Ingredients: ings})
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
			writeRepoError(c, err, "recipe not found", "could not delete recipe")
			return
		}
		doctor, err := doctorOf(c, repo, existing.DoctorID)
		if err != nil {
			return
		}
		user, _ := auth.UserFrom(c)
		if !auth.CanWriteDistrict(user, doctor.DistrictID) {
			httpx.Err(c, http.StatusForbidden, "forbidden", "cannot write to this district")
			return
		}
		if err := repo.Delete(id); err != nil {
			writeRepoError(c, err, "recipe not found", "could not delete recipe")
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func resolveDoctorHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		name := c.Query("name")
		doctorID, mismatch, err := repo.ResolveDoctor(code, name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				httpx.Err(c, http.StatusNotFound, "not_found", "doctor not found")
				return
			}
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not resolve doctor")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"doctor_id": doctorID, "mismatch": mismatch})
	}
}

// doctorOf loads the doctor with id via repo's underlying DB access. On
// failure it writes the appropriate error response and returns a non-nil
// error; callers must return immediately in that case.
func doctorOf(c *gin.Context, repo *Repo, id uint) (model.Doctor, error) {
	var d model.Doctor
	err := repo.g.First(&d, id).Error
	if err != nil {
		writeRepoError(c, err, "doctor not found", "could not load doctor")
	}
	return d, err
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

// parseDoctorQuery parses the required doctor_id query parameter, writing a
// 400 response and returning ok=false on failure.
func parseDoctorQuery(c *gin.Context) (doctorID uint, ok bool) {
	parsed, err := strconv.ParseUint(c.Query("doctor_id"), 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "doctor_id query parameter is required")
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
