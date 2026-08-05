// Command server runs the phum-panya API and static frontend. Subcommands:
//
//	server                      run in the foreground (default)
//	server run                  run under the host service manager
//	server service install      register the OS service (Windows SCM / systemd)
//	server service uninstall    remove the OS service
//	server service start|stop|restart
//	server create-admin         seed the first central admin, then exit
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/backup"
	"phum-panya/internal/bootstrap"
	"phum-panya/internal/clock"
	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/httpx"
	"phum-panya/internal/media"
	"phum-panya/internal/router"
	"phum-panya/internal/svc"
	"phum-panya/internal/telemetry"
)

// sessionTTL is how long a login session stays valid.
const sessionTTL = 7 * 24 * time.Hour

// loginThrottle limits repeated failed logins from the same key.
const loginThrottleMax = 5

const loginThrottleWindow = 15 * time.Minute

// backupInterval is how often the daily backup ticker runs.
const backupInterval = 24 * time.Hour

// backupKeep is how many dated backup zips are retained.
const backupKeep = 14

func main() {
	args := os.Args[1:]
	switch {
	case len(args) >= 1 && args[0] == "create-admin":
		runCreateAdmin()
	case len(args) >= 1 && args[0] == "migrate":
		runMigrate()
	case len(args) >= 1 && args[0] == "service":
		runServiceControl(args[1:])
	default: // "" (foreground) or "run" (under the service manager)
		runServer()
	}
}

// runServiceControl installs, uninstalls, or controls the OS service. The
// install action accepts seed flags (--admin-email, --admin-password,
// --domain, --http-addr) that the MSI passes from its wizard page.
func runServiceControl(args []string) {
	if len(args) < 1 {
		slog.Error("usage: server service install|uninstall|start|stop|restart")
		os.Exit(1)
	}
	action := args[0]
	switch action {
	case "install":
		applyInstallFlags(args[1:])
		control(action)
	case "uninstall", "start", "stop", "restart":
		if len(args) != 1 {
			slog.Error("service takes no arguments", "action", action)
			os.Exit(1)
		}
		control(action)
	default:
		slog.Error("unknown service action", "action", action)
		os.Exit(1)
	}
}

func control(action string) {
	if err := svc.Control(action); err != nil {
		slog.Error("service action failed", "action", action, "err", err)
		os.Exit(1)
	}
	fmt.Printf("service %s: ok\n", action)
}

// applyInstallFlags maps install-time flags to APP_* environment variables so
// they are baked into the service definition (svc.Config reads the environment
// when it registers the service).
func applyInstallFlags(args []string) {
	fs := flag.NewFlagSet("service install", flag.ExitOnError)
	email := fs.String("admin-email", "", "seed the first admin with this email")
	password := fs.String("admin-password", "", "seed the first admin with this password")
	domain := fs.String("domain", "", "public domain for built-in TLS (blank = plain HTTP)")
	addr := fs.String("http-addr", "", "listen address (default :8080)")
	if err := fs.Parse(args); err != nil {
		slog.Error("service install", "err", err)
		os.Exit(1)
	}
	setEnvIf("APP_ADMIN_EMAIL", *email)
	setEnvIf("APP_ADMIN_PASSWORD", *password)
	setEnvIf("APP_DOMAIN", *domain)
	setEnvIf("APP_HTTP_ADDR", *addr)
}

func setEnvIf(key, val string) {
	if val != "" {
		_ = os.Setenv(key, val)
	}
}

// runServer opens the database, ensures the schema and first admin exist,
// wires the engine, starts the daily backup ticker, and serves the app under
// the service supervisor (foreground or OS service manager).
func runServer() {
	cfg := config.Load()
	logger := telemetry.NewLogger(cfg)
	slog.SetDefault(logger)

	if err := ensureDataDirs(cfg); err != nil {
		slog.Error("create data dirs", "err", err)
		os.Exit(1)
	}
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	if cfg.AutoMigrate {
		if err := migrateDB(g, cfg); err != nil {
			slog.Error("migrate", "err", err)
			os.Exit(1)
		}
	}

	mediaStore, err := newMediaStore(context.Background(), cfg)
	if err != nil {
		slog.Error("media store", "err", err)
		os.Exit(1)
	}

	metricsHandler, shutdownTelemetry, err := telemetry.Setup(context.Background(), cfg)
	if err != nil {
		slog.Error("telemetry setup", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()
	if err := telemetry.InstrumentDB(g); err != nil {
		slog.Error("instrument db", "err", err)
		os.Exit(1)
	}

	clk := clock.Real{}
	deps := router.Deps{
		Cfg:        cfg,
		DB:         g,
		Store:      auth.NewSessionStore(g, clk, sessionTTL),
		Throttle:   newLimiter(g, cfg, clk),
		Media:      mediaStore,
		Clk:        clk,
		Logger:     logger,
		Metrics:    metricsHandler,
		Secure:     cfg.CookieSecure(),
		BackupDir:  cfg.BackupDir,
		BackupKeep: backupKeep,
		DBPath:     cfg.DBPath,
		MediaDir:   cfg.MediaDir,
	}
	engine := router.NewEngine(deps)

	if cfg.BackupEnabled() {
		go runBackupTicker(cfg, clk)
	}

	slog.Info("listening", "addr", cfg.HTTPAddr)
	if err := svc.Run(func(ctx context.Context) error {
		return httpx.ServeContext(ctx, cfg, engine)
	}); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}

// newLimiter builds the login rate limiter selected by cfg.ThrottleStore:
// a shared DB-backed limiter for the multi-replica stack, or the in-process
// throttle otherwise.
func newLimiter(g *gorm.DB, cfg config.Config, clk clock.Clock) auth.Limiter {
	if cfg.UsesDBThrottle() {
		return auth.NewDBLimiter(g, clk, loginThrottleMax, loginThrottleWindow)
	}
	return auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow)
}

// newMediaStore builds the media adapter selected by cfg.MediaDriver.
func newMediaStore(ctx context.Context, cfg config.Config) (media.Store, error) {
	if cfg.UsesS3Media() {
		return media.NewS3Store(ctx, cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UsePathStyle)
	}
	return media.NewLocalStore(cfg.MediaDir), nil
}

// ensureDataDirs creates the parent directory of the database file and the
// media and backup directories, so a fresh install (or a service whose
// working directory is empty) can open the database and store files.
func ensureDataDirs(cfg config.Config) error {
	for _, dir := range []string{filepath.Dir(cfg.DBPath), cfg.MediaDir, cfg.BackupDir} {
		if dir == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// runBackupTicker runs a backup once a day for the lifetime of the process,
// logging (but not exiting on) any failure.
func runBackupTicker(cfg config.Config, clk clock.Clock) {
	ticker := time.NewTicker(backupInterval)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := backup.Run(cfg.DBPath, cfg.MediaDir, cfg.BackupDir, backupKeep, clk); err != nil {
			slog.Error("daily backup failed", "err", err)
		}
	}
}

// migrateDB runs the schema migration, the recipe-photo backfill, and the
// first-admin seed. It is the release step: the migrate job runs it once, and
// runServer runs it at startup only when APP_AUTO_MIGRATE is true.
func migrateDB(g *gorm.DB, cfg config.Config) error {
	if err := db.AutoMigrate(g); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := db.BackfillRecipePhotos(g); err != nil {
		return fmt.Errorf("backfill recipe photos: %w", err)
	}
	if _, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}
	return nil
}

// runMigrate is the `server migrate` subcommand: the discrete release step for
// the multi-replica stack.
func runMigrate() {
	cfg := config.Load()
	slog.SetDefault(telemetry.NewLogger(cfg))
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	if err := migrateDB(g, cfg); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("migrate: done")
}

// runCreateAdmin seeds the first central admin from APP_ADMIN_EMAIL and
// APP_ADMIN_PASSWORD, then exits.
func runCreateAdmin() {
	cfg := config.Load()
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	if err := db.AutoMigrate(g); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	created, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		slog.Error("ensure admin", "err", err)
		os.Exit(1)
	}
	if created {
		fmt.Println("admin created")
	} else {
		fmt.Println("admin already exists or nothing to seed")
	}
}
