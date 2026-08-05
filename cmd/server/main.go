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
	"log"
	"os"
	"path/filepath"
	"time"

	"phum-panya/internal/auth"
	"phum-panya/internal/backup"
	"phum-panya/internal/bootstrap"
	"phum-panya/internal/clock"
	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/httpx"
	"phum-panya/internal/media"
	"phum-panya/internal/model"
	"phum-panya/internal/router"
	"phum-panya/internal/svc"
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
		log.Fatal("usage: server service install|uninstall|start|stop|restart")
	}
	action := args[0]
	switch action {
	case "install":
		applyInstallFlags(args[1:])
		control(action)
	case "uninstall", "start", "stop", "restart":
		if len(args) != 1 {
			log.Fatalf("service %s takes no arguments", action)
		}
		control(action)
	default:
		log.Fatalf("unknown service action %q (install|uninstall|start|stop|restart)", action)
	}
}

func control(action string) {
	if err := svc.Control(action); err != nil {
		log.Fatalf("service %s: %v", action, err)
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
		log.Fatalf("service install: %v", err)
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
	if err := ensureDataDirs(cfg); err != nil {
		log.Fatalf("create data dirs: %v", err)
	}
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := model.BackfillRecipePhotos(g); err != nil {
		log.Fatalf("backfill recipe photos: %v", err)
	}
	if _, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	clk := clock.Real{}
	deps := router.Deps{
		Cfg:        cfg,
		DB:         g,
		Store:      auth.NewSessionStore(g, clk, sessionTTL),
		Throttle:   auth.NewThrottle(clk, loginThrottleMax, loginThrottleWindow),
		Media:      &media.Store{Dir: cfg.MediaDir},
		Clk:        clk,
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

	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := svc.Run(func(ctx context.Context) error {
		return httpx.ServeContext(ctx, cfg, engine)
	}); err != nil {
		log.Fatalf("serve: %v", err)
	}
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
			log.Printf("daily backup failed: %v", err)
		}
	}
}

// runCreateAdmin seeds the first central admin from APP_ADMIN_EMAIL and
// APP_ADMIN_PASSWORD, then exits.
func runCreateAdmin() {
	cfg := config.Load()
	g, err := db.OpenWith(cfg.DBDriver, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	created, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	if created {
		fmt.Println("admin created")
	} else {
		fmt.Println("admin already exists or nothing to seed")
	}
}
