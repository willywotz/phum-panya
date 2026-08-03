package main

import (
	"os"
	"testing"
)

func TestApplyInstallFlagsMapsToEnv(t *testing.T) {
	for _, k := range []string{"APP_ADMIN_EMAIL", "APP_ADMIN_PASSWORD", "APP_DOMAIN", "APP_HTTP_ADDR"} {
		t.Setenv(k, "")
	}

	applyInstallFlags([]string{
		"--admin-email=admin@example.com",
		"--admin-password=secret-pw",
		"--domain=example.com",
	})

	if got := os.Getenv("APP_ADMIN_EMAIL"); got != "admin@example.com" {
		t.Fatalf("APP_ADMIN_EMAIL = %q", got)
	}
	if got := os.Getenv("APP_ADMIN_PASSWORD"); got != "secret-pw" {
		t.Fatalf("APP_ADMIN_PASSWORD = %q", got)
	}
	if got := os.Getenv("APP_DOMAIN"); got != "example.com" {
		t.Fatalf("APP_DOMAIN = %q", got)
	}
}

func TestApplyInstallFlagsLeavesUnsetEnvEmpty(t *testing.T) {
	t.Setenv("APP_HTTP_ADDR", "")
	applyInstallFlags([]string{"--admin-email=a@b.co"})
	if got := os.Getenv("APP_HTTP_ADDR"); got != "" {
		t.Fatalf("APP_HTTP_ADDR should stay empty, got %q", got)
	}
}
