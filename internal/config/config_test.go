package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	c := Load()

	if c.DBPath == "" {
		t.Error("DBPath should not be empty by default")
	}
	if c.MediaDir == "" {
		t.Error("MediaDir should not be empty by default")
	}
	if c.BackupDir == "" {
		t.Error("BackupDir should not be empty by default")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("APP_DOMAIN", "example.org")
	t.Setenv("APP_DEV", "1")
	t.Setenv("APP_ADMIN_EMAIL", "admin@example.com")

	c := Load()

	if c.Domain != "example.org" {
		t.Errorf("Domain: got %q, want %q", c.Domain, "example.org")
	}
	if !c.DevMode {
		t.Errorf("DevMode: got %v, want true", c.DevMode)
	}
	if c.AdminEmail != "admin@example.com" {
		t.Errorf("AdminEmail: got %q, want %q", c.AdminEmail, "admin@example.com")
	}
}
