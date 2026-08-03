// Package router assembles the full gin.Engine from every module's routes.
// It cannot live in internal/httpx: every module it wires (auth, district,
// user, herb, doctor, recipe, caserec, publicapi, export, backup, webui)
// imports internal/httpx for httpx.OK/httpx.Err, so a package httpx that
// imported them back would be a cyclic import.
package router

import (
	"net/http"

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
	"phum-panya/internal/media"
	"phum-panya/internal/publicapi"
	"phum-panya/internal/recipe"
	"phum-panya/internal/user"
	"phum-panya/internal/webui"
)

// Deps bundles everything NewEngine needs to assemble the API.
type Deps struct {
	Cfg   config.Config
	DB    *gorm.DB
	Store *auth.SessionStore

	Throttle *auth.Throttle
	Media    *media.Store
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
	engine.Use(auth.SameOrigin(deps.Cfg.Domain))

	// api carries auth.LoadUser but no path prefix: every module already
	// registers its full "/api/..." literal path, so a "/api" group prefix
	// here would double it.
	api := engine.Group("")
	api.Use(auth.LoadUser(deps.Store, deps.DB))
	api.GET("/api/health", func(c *gin.Context) { httpx.OK(c, http.StatusOK, gin.H{"status": "ok"}) })

	auth.RegisterRoutes(api, deps.DB, deps.Store, deps.Throttle, deps.Secure)
	district.RegisterRoutes(api, district.NewRepo(deps.DB))
	user.RegisterRoutes(api, user.NewRepo(deps.DB))
	herb.RegisterRoutes(api, herb.NewRepo(deps.DB), deps.Media)
	doctor.RegisterRoutes(api, doctor.NewRepo(deps.DB, deps.Clk), deps.Media)
	recipe.RegisterRoutes(api, recipe.NewRepo(deps.DB, deps.Clk))
	caserec.RegisterRoutes(api, caserec.NewRepo(deps.DB, deps.Clk), deps.Media)
	publicapi.RegisterRoutes(api, deps.DB)
	export.RegisterRoutes(api, deps.DB)
	backup.RegisterRoutes(api, deps.DBPath, deps.MediaDir, deps.BackupDir, deps.BackupKeep, deps.Clk)

	webui.Register(engine)

	return engine
}
