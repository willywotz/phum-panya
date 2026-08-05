package config

import (
	"net/url"
	"os"
)

type Config struct {
	HTTPAddr      string
	Domain        string
	DBDriver      string
	DBPath        string
	DatabaseURL   string
	MediaDir      string
	BackupDir     string
	DevMode       bool
	BehindProxy   bool
	PublicOrigin  string
	AdminEmail    string
	AdminPassword string
}

func Load() Config {
	return Config{
		HTTPAddr:      env("APP_HTTP_ADDR", ":8080"),
		Domain:        env("APP_DOMAIN", ""),
		DBDriver:      env("APP_DB_DRIVER", "sqlite"),
		DBPath:        env("APP_DB_PATH", "data/app.db"),
		DatabaseURL:   env("APP_DATABASE_URL", ""),
		MediaDir:      env("APP_MEDIA_DIR", "data/media"),
		BackupDir:     env("APP_BACKUP_DIR", "data/backup"),
		DevMode:       os.Getenv("APP_DEV") == "1",
		BehindProxy:   os.Getenv("APP_BEHIND_PROXY") == "1",
		PublicOrigin:  env("APP_PUBLIC_ORIGIN", ""),
		AdminEmail:    env("APP_ADMIN_EMAIL", ""),
		AdminPassword: env("APP_ADMIN_PASSWORD", ""),
	}
}

// UseTLS reports whether the server terminates TLS itself via autocert.
// False in dev, behind a reverse proxy, or when no domain is set.
func (c Config) UseTLS() bool {
	return !c.DevMode && !c.BehindProxy && c.Domain != ""
}

// CookieSecure reports whether the session cookie carries the Secure flag.
// True under production TLS and behind a proxy (public scheme is HTTPS).
func (c Config) CookieSecure() bool {
	return c.BehindProxy || (!c.DevMode && c.Domain != "")
}

// AllowedOriginHost returns the host the CSRF origin check must match.
// Behind a proxy it is the host of PublicOrigin; otherwise it is Domain.
func (c Config) AllowedOriginHost() string {
	if c.BehindProxy && c.PublicOrigin != "" {
		if u, err := url.Parse(c.PublicOrigin); err == nil {
			return u.Host
		}
	}
	return c.Domain
}

// BackupEnabled reports whether the in-app SQLite backup loop runs.
// It is disabled on Postgres, where the pg_dump sidecar owns backups.
func (c Config) BackupEnabled() bool {
	return c.DBDriver != "postgres"
}

func env(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}
