package config

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

// Config holds all environment-driven settings.
type Config struct {
	// Server
	Port string

	// Auth
	AuthUsername string
	AuthPassword string

	// Discord
	DiscordToken      string
	ReportChannelID   int64
	CommandPrefix     string
	AutoThreadArchive int

	// Music APIs
	PopularityThreshold int
	LastFMAPIKey        string

	// Lidarr
	LidarrURL            string
	LidarrAPIKey         string
	LidarrQualityProfile string
	LidarrRootFolder     string

	// Download
	DownloadMode string // "tracks" or "album"

	// Scheduler
	DailyCheckCron string
	Timezone       string

	// Database
	DBPath string
}

// Load reads env vars and returns a Config.
func Load() *Config {
	c := &Config{
		Port:                 getEnv("PORT", "8080"),
		AuthUsername:         getEnv("AUTH_USERNAME", "admin"),
		AuthPassword:         getEnv("AUTH_PASSWORD", "changeme"),
		DiscordToken:         getEnv("DISCORD_TOKEN", ""),
		CommandPrefix:        getEnv("COMMAND_PREFIX", "?"),
		AutoThreadArchive:    10080, // 7 days in minutes
		PopularityThreshold:  intEnv("POPULARITY_THRESHOLD", 60),
		LastFMAPIKey:         getEnv("LASTFM_API_KEY", ""),
		LidarrURL:            getEnv("LIDARR_URL", "http://localhost:8686"),
		LidarrAPIKey:         getEnv("LIDARR_API_KEY", ""),
		LidarrQualityProfile: getEnv("LIDARR_QUALITY_PROFILE", "Standard"),
		LidarrRootFolder:     getEnv("LIDARR_ROOT_FOLDER", ""),
		DownloadMode:         getEnv("DOWNLOAD_MODE", "tracks"),
		DailyCheckCron:       getEnv("DAILY_CHECK_CRON", "0 9 * * *"),
		Timezone:             getEnv("TIMEZONE", "America/Detroit"),
		DBPath:               getEnv("DB_PATH", "/data/groovarr.db"),
	}

	if v := os.Getenv("REPORT_CHANNEL_ID"); v != "" {
		c.ReportChannelID, _ = strconv.ParseInt(v, 10, 64)
	}

	return c
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

// DefaultHTTPClient is a shared HTTP client with timeouts.
var DefaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
