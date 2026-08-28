package config

import (
	"os"
)

// Config holds the essential environment-driven settings.
// All other configuration is stored in the database.
type Config struct {
	Port        string
	AuthUsername string
	AuthPassword string
	DBPath      string
	DBSalt      string // optional, for future use (e.g., DB encryption)
}

// Load reads only essential env vars.
// All other configuration is loaded from the database via the settings service.
func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		AuthUsername: getEnv("AUTH_USERNAME", ""),
		AuthPassword: getEnv("AUTH_PASSWORD", ""),
		DBPath:      getEnv("DB_PATH", "/data/groovarr.db"),
		DBSalt:      getEnv("DB_SALT", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}