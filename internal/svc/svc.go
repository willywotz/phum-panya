// Package svc runs the server under the host service manager (Windows SCM,
// Linux systemd, macOS launchd) via kardianos/service, and installs or
// uninstalls that service. It is a thin supervisor: the caller supplies the
// run function, so this package stays free of the app's wiring.
package svc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
)

const (
	serviceName = "phum-panya"
	displayName = "phum-panya"
	description = "phum-panya folk-medicine records server"
)

// configEnvKeys are the APP_* variables baked into the service environment at
// install time, so the service manager starts the server with the same config
// the installer collected.
var configEnvKeys = []string{
	"APP_HTTP_ADDR", "APP_DOMAIN", "APP_DB_PATH", "APP_MEDIA_DIR",
	"APP_BACKUP_DIR", "APP_DEV", "APP_ADMIN_EMAIL", "APP_ADMIN_PASSWORD",
}

// DataDir is the per-platform working directory for the service. All relative
// data paths (app.db, media, backups, certs) resolve under it, so the service
// does not depend on the launcher's current directory.
func DataDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "phum-panya")
	}
	return "/var/lib/phum-panya"
}

// Config builds the kardianos service definition, baking the current non-empty
// APP_* environment (the installer's collected values) into the service.
func Config() *service.Config {
	env := map[string]string{}
	for _, key := range configEnvKeys {
		if val := os.Getenv(key); val != "" {
			env[key] = val
		}
	}
	return &service.Config{
		Name:             serviceName,
		DisplayName:      displayName,
		Description:      description,
		Arguments:        []string{"run"},
		WorkingDirectory: DataDir(),
		EnvVars:          env,
	}
}

// program adapts a run function to the kardianos service interface: Start
// launches it in the background, Stop cancels its context and waits.
type program struct {
	run    func(context.Context) error
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		p.err = p.run(ctx)
		close(p.done)
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		<-p.done
	}
	return p.err
}

// Run supervises run under the service manager: in the foreground it blocks
// until interrupted; under a service manager it responds to start and stop.
func Run(run func(context.Context) error) error {
	s, err := service.New(&program{run: run}, Config())
	if err != nil {
		return err
	}
	return s.Run()
}

// Control performs a service lifecycle action: install, uninstall, start,
// stop, or restart. Install first ensures the working directory exists.
func Control(action string) error {
	if action == "install" {
		if err := os.MkdirAll(DataDir(), 0o755); err != nil {
			return err
		}
	}
	s, err := service.New(&program{run: func(context.Context) error { return nil }}, Config())
	if err != nil {
		return err
	}
	return service.Control(s, action)
}
