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
	"fmt"
	"log"
	"os"
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

// runServiceControl installs, uninstalls, or controls the OS service.
func runServiceControl(args []string) {
	if len(args) != 1 {
		log.Fatal("usage: server service install|uninstall|start|stop|restart")
	}
	action := args[0]
	switch action {
	case "install", "uninstall", "start", "stop", "restart":
		if err := svc.Control(action); err != nil {
			log.Fatalf("service %s: %v", action, err)
		}
		fmt.Printf("service %s: ok\n", action)
	default:
		log.Fatalf("unknown service action %q (install|uninstall|start|stop|restart)", action)
	}
}

// runServer opens the database, ensures the schema and first admin exist,
// wires the engine, starts the daily backup ticker, and serves the app under
// the service supervisor (foreground or OS service manager).
func runServer() {
	cfg := config.Load()
	g, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		log.Fatalf("migrate: %v", err)
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
		Secure:     !cfg.DevMode && cfg.Domain != "",
		BackupDir:  cfg.BackupDir,
		BackupKeep: backupKeep,
		DBPath:     cfg.DBPath,
		MediaDir:   cfg.MediaDir,
	}
	engine := router.NewEngine(deps)

	go runBackupTicker(cfg, clk)

	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := svc.Run(func(ctx context.Context) error {
		return httpx.ServeContext(ctx, cfg, engine)
	}); err != nil {
		log.Fatalf("serve: %v", err)
	}
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
	g, err := db.Open(cfg.DBPath)
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
