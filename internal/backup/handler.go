package backup

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/httpx"
)

// RegisterRoutes wires POST /api/backup/run, restricted to central_admin,
// which runs Run(dbPath, mediaDir, outDir, keep, clk) on demand.
func RegisterRoutes(r gin.IRouter, dbPath, mediaDir, outDir string, keep int, clk clock.Clock) {
	r.POST("/api/backup/run", auth.RequireRole("central_admin"), func(c *gin.Context) {
		zipPath, err := Run(dbPath, mediaDir, outDir, keep, clk)
		if err != nil {
			httpx.Err(c, http.StatusInternalServerError, "backup_failed", err.Error())
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"zip": zipPath})
	})
}
