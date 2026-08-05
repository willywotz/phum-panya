package config

import (
	"os"
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

func TestModePredicates(t *testing.T) {
	cases := []struct {
		name       string
		cfg        Config
		wantTLS    bool
		wantSecure bool
		wantHost   string
		wantBackup bool
	}{
		{"dev", Config{DevMode: true}, false, false, "", true},
		{"prod-tls", Config{Domain: "example.org"}, true, true, "example.org", true},
		{"behind-proxy", Config{BehindProxy: true, PublicOrigin: "https://example.org"}, false, true, "example.org", true},
		{"behind-proxy-http", Config{BehindProxy: true, PublicOrigin: "http://localhost:8080"}, false, false, "localhost:8080", true},
		{"postgres", Config{BehindProxy: true, PublicOrigin: "https://example.org", DBDriver: "postgres"}, false, true, "example.org", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.UseTLS(); got != tc.wantTLS {
				t.Errorf("UseTLS() = %v, want %v", got, tc.wantTLS)
			}
			if got := tc.cfg.CookieSecure(); got != tc.wantSecure {
				t.Errorf("CookieSecure() = %v, want %v", got, tc.wantSecure)
			}
			if got := tc.cfg.AllowedOriginHost(); got != tc.wantHost {
				t.Errorf("AllowedOriginHost() = %q, want %q", got, tc.wantHost)
			}
			if got := tc.cfg.BackupEnabled(); got != tc.wantBackup {
				t.Errorf("BackupEnabled() = %v, want %v", got, tc.wantBackup)
			}
		})
	}
}

func TestLoadNewDefaults(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.DBDriver != "sqlite" {
		t.Errorf("DBDriver default = %q, want sqlite", c.DBDriver)
	}
	if c.BehindProxy {
		t.Errorf("BehindProxy default = true, want false")
	}
}
