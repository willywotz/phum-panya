package importer

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// importerService is the port RegisterRoutes depends on; *Importer is the
// concrete implementation.
type importerService interface {
	DryRun(r io.Reader, sourceName string) (*Report, error)
	Run(r io.Reader, sourceName string, actorID uint) (*Report, error)
	Undo(batchID uint) error
}

// RegisterRoutes wires the central-admin importer endpoints: a multipart
// workbook upload (dry-run or commit via ?dryRun=true) and per-batch undo.
func RegisterRoutes(r gin.IRouter, im importerService) {
	admin := auth.RequireRole(model.RoleCentralAdmin)
	r.POST("/api/imports", admin, importHandler(im))
	r.POST("/api/imports/:batchId/undo", admin, undoHandler(im))
}

func importHandler(im importerService) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := auth.UserFrom(c)
		fh, err := c.FormFile("file")
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "file_required", "an .xlsx file upload named \"file\" is required")
			return
		}
		f, err := fh.Open()
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "file_open_failed", err.Error())
			return
		}
		defer f.Close()

		var rep *Report
		if c.Query("dryRun") == "true" {
			rep, err = im.DryRun(f, fh.Filename)
		} else {
			rep, err = im.Run(f, fh.Filename, user.ID)
		}
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "import_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, rep)
	}
}

func undoHandler(im importerService) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("batchId"), 10, 64)
		if err != nil {
			httpx.Err(c, http.StatusBadRequest, "bad_id", "invalid batch id")
			return
		}
		if err := im.Undo(uint(id)); err != nil {
			httpx.Err(c, http.StatusInternalServerError, "undo_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "undone", "batchId": id})
	}
}
