// Package router assembles the full gin.Engine from every module's routes.
// It cannot live in internal/httpx: every module it wires (auth, district,
// user, herb, doctor, recipe, caserec, publicapi, export, backup, webui)
// imports internal/httpx for httpx.OK/httpx.Err, so a package httpx that
// imported them back would be a cyclic import.
package router

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/backup"
	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/config"
	"phum-panya/internal/district"
	"phum-panya/internal/doctor"
	"phum-panya/internal/export"
	"phum-panya/internal/herb"
	"phum-panya/internal/httpx"
	"phum-panya/internal/importer"
	"phum-panya/internal/media"
	"phum-panya/internal/publicapi"
	"phum-panya/internal/recipe"
	"phum-panya/internal/review"
	"phum-panya/internal/revision"
	"phum-panya/internal/user"
	"phum-panya/internal/webui"
	"phum-panya/internal/yearlock"
)

// Deps bundles everything NewEngine needs to assemble the API.
type Deps struct {
	Cfg   config.Config
	DB    *gorm.DB
	Store auth.Sessions

	Throttle auth.Limiter
	Media    media.Store
	Clk      clock.Clock
	// Secure controls whether the session cookie is marked Secure.
	Secure bool

	// Backup parameters, forwarded to backup.RegisterRoutes.
	BackupDir  string
	BackupKeep int
	DBPath     string
	MediaDir   string
}

// NewEngine assembles the full gin.Engine: recovery/logging middleware,
// same-origin CSRF defense, every module's routes under /api (with
// auth.LoadUser attaching the current user), and the SPA/asset/JSON-404
// fallback for everything else.
func NewEngine(deps Deps) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())
	engine.MaxMultipartMemory = 8 << 20
	engine.Use(auth.SameOrigin(deps.Cfg.AllowedOriginHost()))

	// api carries auth.LoadUser but no path prefix: every module already
	// registers its full "/api/..." literal path, so a "/api" group prefix
	// here would double it.
	authUsers := auth.NewGormUsers(deps.DB)

	api := engine.Group("")
	api.Use(auth.LoadUser(deps.Store, authUsers))
	api.GET("/api/health", func(c *gin.Context) { httpx.OK(c, http.StatusOK, gin.H{"status": "ok"}) })

	rev := revision.NewRepo(deps.DB, deps.Clk)
	// lockRepo is also used by the yearlock routes registered below.
	lockRepo := yearlock.NewRepo(deps.DB, deps.Clk)

	auth.RegisterRoutes(api, authUsers, deps.Store, deps.Throttle, deps.Secure)
	district.RegisterRoutes(api, district.NewRepo(deps.DB))
	user.RegisterRoutes(api, user.NewRepo(deps.DB))
	herbRepo := herb.NewRepo(deps.DB)
	docRepo := doctor.NewRepo(deps.DB, deps.Clk, rev)
	recRepo := recipe.NewRepo(deps.DB, deps.Clk, rev, lockRepo)
	casRepo := caserec.NewRepo(deps.DB, deps.Clk, rev, lockRepo)

	herb.RegisterRoutes(api, herbRepo, deps.Media)
	doctor.RegisterRoutes(api, docRepo, deps.Media)
	recipe.RegisterRoutes(api, recRepo, deps.Media)
	caserec.RegisterRoutes(api, casRepo, deps.Media)
	review.RegisterRoutes(api, review.NewRepo(deps.DB, rev))
	publicapi.RegisterRoutes(api, publicapi.NewRepo(deps.DB))
	export.RegisterRoutes(api, export.NewSource(deps.DB))
	if deps.Cfg.BackupEnabled() {
		backup.RegisterRoutes(api, deps.DBPath, deps.MediaDir, deps.BackupDir, deps.BackupKeep, deps.Clk)
	}
	yearlock.RegisterRoutes(api, lockRepo)
	importer.RegisterRoutes(api, importer.NewImporter(deps.DB, deps.Clk, docRepo, recRepo, casRepo, herbRepo, lockRepo))

	// A local-driver media dir may not exist yet; create it so the local
	// adapter can serve/write. Harmless (and skipped) when MediaDir is empty
	// (s3 driver). Serving no longer depends on the dir — Open maps a missing
	// file to a 404.
	if deps.Cfg.MediaDir != "" {
		_ = os.MkdirAll(deps.Cfg.MediaDir, 0o755)
	}
	// Public, no-auth; registered before webui.Register so its SPA catch-all
	// never shadows a photo. The handler streams from whichever media adapter
	// is wired (LocalStore or S3Store/Garage).
	engine.GET("/media/*key", mediaHandler(deps.Media))

	webui.Register(engine)

	return engine
}
