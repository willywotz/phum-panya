package publicapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/httpx"
)

// recipeWithCases composes a public Recipe with its public Cases.
type recipeWithCases struct {
	Recipe
	Cases []Case `json:"cases"`
}

// doctorDetail composes a public Doctor with its recipes (each with its
// cases and attribution).
type doctorDetail struct {
	Doctor
	Recipes []recipeWithCases `json:"recipes"`
}

// RegisterRoutes wires the public, read-only, no-auth routes onto r.
func RegisterRoutes(r gin.IRouter, g *gorm.DB) {
	repo := NewRepo(g)
	r.GET("/api/public/doctors", listDoctorsHandler(repo))
	r.GET("/api/public/doctors/:id", doctorDetailHandler(repo))
	r.GET("/api/public/recipes", listRecipesHandler(repo))
	r.GET("/api/public/herbs", listHerbsHandler(repo))
}

func listDoctorsHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		districtID, ok := parseOptionalID(c, "district_id")
		if !ok {
			return
		}
		doctors, err := repo.ListDoctors(DoctorFilter{Q: c.Query("q"), DistrictID: districtID})
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list doctors")
			return
		}
		httpx.OK(c, http.StatusOK, doctors)
	}
}

func doctorDetailHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		doc, err := repo.GetDoctor(id)
		if err != nil {
			writeRepoError(c, err, "doctor not found", "could not load doctor")
			return
		}
		recipes, err := repo.ListRecipesByDoctor(id)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list recipes")
			return
		}
		out := doctorDetail{Doctor: doc, Recipes: make([]recipeWithCases, len(recipes))}
		for i, rec := range recipes {
			cases, err := repo.ListCasesByRecipe(rec.ID)
			if err != nil {
				httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list cases")
				return
			}
			out.Recipes[i] = recipeWithCases{Recipe: rec, Cases: cases}
		}
		httpx.OK(c, http.StatusOK, out)
	}
}

func listRecipesHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		districtID, ok := parseOptionalID(c, "district_id")
		if !ok {
			return
		}
		herbID, ok := parseOptionalID(c, "herb_id")
		if !ok {
			return
		}
		recipes, err := repo.ListRecipes(RecipeFilter{Q: c.Query("q"), DistrictID: districtID, HerbID: herbID})
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list recipes")
			return
		}
		httpx.OK(c, http.StatusOK, recipes)
	}
}

func listHerbsHandler(repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		herbs, err := repo.ListHerbs()
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "internal_error", "could not list herbs")
			return
		}
		httpx.OK(c, http.StatusOK, herbs)
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

// parseOptionalID parses the query parameter name as a uint, if present. It
// returns id=nil, ok=true when the parameter is absent. A malformed value
// writes a 400 response and returns ok=false.
func parseOptionalID(c *gin.Context, name string) (id *uint, ok bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid "+name)
		return nil, false
	}
	val := uint(parsed)
	return &val, true
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
