package export

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
)

const (
	contentTypeCSV  = "text/csv"
	contentTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// exportFunc is the shape shared by Doctors, Recipes, and Cases.
type exportFunc func(w io.Writer, g *gorm.DB, format string, districtID *uint) error

// RegisterRoutes wires the staff-only bulk export endpoints onto r. Every
// route requires authentication. A district_editor's district is forced
// from their session (any district_id query parameter is ignored); a
// central_admin may pass district_id or omit it to export every district.
func RegisterRoutes(r gin.IRouter, g *gorm.DB) {
	requireAuth := auth.RequireAuth()
	for _, e := range []struct {
		name string
		fn   exportFunc
	}{
		{"doctors", Doctors},
		{"recipes", Recipes},
		{"cases", Cases},
	} {
		r.GET("/api/export/"+e.name+".csv", requireAuth, exportHandler(g, "csv", e.fn, e.name))
		r.GET("/api/export/"+e.name+".xlsx", requireAuth, exportHandler(g, "xlsx", e.fn, e.name))
	}
}

func exportHandler(g *gorm.DB, format string, fn exportFunc, name string) gin.HandlerFunc {
	contentType := contentTypeCSV
	if format == "xlsx" {
		contentType = contentTypeXLSX
	}
	return func(c *gin.Context) {
		districtID, ok := scopedDistrict(c)
		if !ok {
			return
		}
		c.Writer.Header().Set("Content-Type", contentType)
		c.Writer.Header().Set("Content-Disposition", "attachment; filename="+name+"."+format)
		c.Writer.WriteHeader(http.StatusOK)
		if err := fn(c.Writer, g, format, districtID); err != nil {
			return
		}
	}
}

// scopedDistrict returns the district a request may export from: a
// district_editor is forced to their own district; a central_admin may pass
// ?district_id= or omit it (nil = every district). It writes a 400 response
// and returns ok=false for a malformed district_id.
func scopedDistrict(c *gin.Context) (*uint, bool) {
	user, _ := auth.UserFrom(c)
	if user.Role == "district_editor" {
		return user.DistrictID, true
	}
	raw := c.Query("district_id")
	if raw == "" {
		return nil, true
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		httpx.Err(c, http.StatusBadRequest, "invalid_request", "invalid district_id")
		return nil, false
	}
	id := uint(parsed)
	return &id, true
}
