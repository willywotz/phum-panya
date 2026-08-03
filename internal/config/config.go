package config

import "os"

type Config struct {
	HTTPAddr      string
	Domain        string
	DBPath        string
	MediaDir      string
	BackupDir     string
	DevMode       bool
	AdminEmail    string
	AdminPassword string
}

func Load() Config {
	return Config{
		HTTPAddr:      env("APP_HTTP_ADDR", ":8080"),
		Domain:        env("APP_DOMAIN", ""),
		DBPath:        env("APP_DB_PATH", "data/app.db"),
		MediaDir:      env("APP_MEDIA_DIR", "data/media"),
		BackupDir:     env("APP_BACKUP_DIR", "data/backup"),
		DevMode:       os.Getenv("APP_DEV") == "1",
		AdminEmail:    env("APP_ADMIN_EMAIL", ""),
		AdminPassword: env("APP_ADMIN_PASSWORD", ""),
	}
}

func env(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}
