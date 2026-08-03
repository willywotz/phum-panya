package svc

import (
	"context"
	"testing"
	"time"
)

func TestConfigBakesEnvAndRunArgument(t *testing.T) {
	t.Setenv("APP_DOMAIN", "example.com")
	t.Setenv("APP_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("APP_ADMIN_PASSWORD", "secret-pw")

	c := Config()
	if c.Name != serviceName {
		t.Fatalf("name = %q, want %q", c.Name, serviceName)
	}
	if len(c.Arguments) != 1 || c.Arguments[0] != "run" {
		t.Fatalf("arguments = %v, want [run]", c.Arguments)
	}
	if c.WorkingDirectory == "" {
		t.Fatal("working directory is empty")
	}
	if c.EnvVars["APP_DOMAIN"] != "example.com" {
		t.Fatalf("APP_DOMAIN not baked: %v", c.EnvVars)
	}
	if c.EnvVars["APP_ADMIN_PASSWORD"] != "secret-pw" {
		t.Fatalf("APP_ADMIN_PASSWORD not baked: %v", c.EnvVars)
	}
}

func TestConfigOmitsUnsetEnv(t *testing.T) {
	t.Setenv("APP_BACKUP_DIR", "")
	if _, ok := Config().EnvVars["APP_BACKUP_DIR"]; ok {
		t.Fatal("an empty env var must not be baked into the service")
	}
}

func TestProgramStartRunsAndStopCancels(t *testing.T) {
	started := make(chan struct{})
	p := &program{run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	}}

	if err := p.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("run function never started")
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("stop returned %v, want nil", err)
	}
}
