package config

import (
	"net/http"
	"os"
	"strconv"
	"strings"
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
	DiscordToken           string
	DiscordHomeChannel    int64
	DiscordAllowedChans   []int64 // empty = allow all
	DiscordAllowedUsers   []string // empty = depends on DiscordAllowAllUsers
	DiscordAllowAllUsers  bool
	DiscordAutoThread     bool
	DiscordRequireMention bool
	CommandPrefix         string

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
		DiscordToken:         getEnv("DISCORD_BOT_TOKEN", ""),
		DiscordAllowAllUsers: boolEnv("DISCORD_ALLOW_ALL_USERS", true),
		DiscordAutoThread:    boolEnv("DISCORD_AUTO_THREAD", false),
		DiscordRequireMention: boolEnv("DISCORD_REQUIRE_MENTION", false),
		CommandPrefix:        getEnv("COMMAND_PREFIX", "?"),
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

	if v := os.Getenv("DISCORD_HOME_CHANNEL"); v != "" {
		c.DiscordHomeChannel, _ = strconv.ParseInt(v, 10, 64)
	}

	if v := os.Getenv("DISCORD_ALLOWED_CHANNELS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if id, err := strconv.ParseInt(p, 10, 64); err == nil {
				c.DiscordAllowedChans = append(c.DiscordAllowedChans, id)
			}
		}
	}

	if v := os.Getenv("DISCORD_ALLOWED_USERS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				c.DiscordAllowedUsers = append(c.DiscordAllowedUsers, p)
			}
		}
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

func boolEnv(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return fallback
}

// DefaultHTTPClient is a shared HTTP client with timeouts.
var DefaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
