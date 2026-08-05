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

// sourceFunc is the shape shared by every Source method.
type sourceFunc func(w io.Writer, format string, districtID *uint) error

// Source writes an export stream for one entity in the given format, scoped
// to districtID (nil = all districts).
type Source interface {
	Doctors(w io.Writer, format string, districtID *uint) error
	Recipes(w io.Writer, format string, districtID *uint) error
	Cases(w io.Writer, format string, districtID *uint) error
}

type gormSource struct{ g *gorm.DB }

// NewSource returns a GORM-backed export Source.
func NewSource(g *gorm.DB) Source { return gormSource{g: g} }

func (s gormSource) Doctors(w io.Writer, format string, d *uint) error {
	return Doctors(w, s.g, format, d)
}
func (s gormSource) Recipes(w io.Writer, format string, d *uint) error {
	return Recipes(w, s.g, format, d)
}
func (s gormSource) Cases(w io.Writer, format string, d *uint) error { return Cases(w, s.g, format, d) }

// RegisterRoutes wires the staff-only bulk export endpoints onto r. Every
// route requires authentication. A district_editor's district is forced
// from their session (any district_id query parameter is ignored); a
// central_admin may pass district_id or omit it to export every district.
func RegisterRoutes(r gin.IRouter, src Source) {
	requireAuth := auth.RequireAuth()
	for _, e := range []struct {
		name string
		fn   sourceFunc
	}{
		{"doctors", src.Doctors},
		{"recipes", src.Recipes},
		{"cases", src.Cases},
	} {
		r.GET("/api/export/"+e.name+".csv", requireAuth, exportHandler("csv", e.fn, e.name))
		r.GET("/api/export/"+e.name+".xlsx", requireAuth, exportHandler("xlsx", e.fn, e.name))
	}
}

func exportHandler(format string, fn sourceFunc, name string) gin.HandlerFunc {
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
		if err := fn(c.Writer, format, districtID); err != nil {
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
