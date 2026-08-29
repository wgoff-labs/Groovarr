package config

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// Config holds all configuration, from env vars and the database.
type Config struct {
	// Server
	Port string

	// Auth (basic, optional - if set, enables login)
	AuthUsername string
	AuthPassword string

	// Discord (optional)
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
	LidarrURL              string
	LidarrAPIKey           string
	LidarrQualityProfile   string
	LidarrRootFolders      []string // optional, scanned from Lidarr if empty
	LidarrDefaultRootFolder string  // which folder to use if none specified per-artist

	// Download
	DownloadMode string // "tracks" or "album"

	// Scheduler
	DailyCheckCron string
	Timezone       string

	// Database
	DBPath string
	DBSalt string // optional, for future use (e.g., DB encryption)
}

// global holds the singleton configuration.
var global *Config

// Load returns the global configuration, initializing it from environment variables if needed.
func Load() *Config {
	if global == nil {
		global = &Config{
			Port:        getEnv("PORT", "8080"),
			AuthUsername: getEnv("AUTH_USERNAME", ""),
			AuthPassword: getEnv("AUTH_PASSWORD", ""),
			DiscordToken:         getEnv("DISCORD_BOT_TOKEN", ""),
			DiscordHomeChannel:   int64(getEnvInt("DISCORD_HOME_CHANNEL", 0)),
			DiscordAllowAllUsers: getEnvBool("DISCORD_ALLOW_ALL_USERS", true),
			DiscordAutoThread:    getEnvBool("DISCORD_AUTO_THREAD", false),
			DiscordRequireMention:getEnvBool("DISCORD_REQUIRE_MENTION", false),
			CommandPrefix:        getEnv("COMMAND_PREFIX", "?"),
			PopularityThreshold:  getEnvInt("POPULARITY_THRESHOLD", 60),
			LastFMAPIKey:         getEnv("LASTFM_API_KEY", ""),
			LidarrURL:            getEnv("LIDARR_URL", "http://localhost:8686"),
			LidarrAPIKey:         getEnv("LIDARR_API_KEY", ""),
			LidarrQualityProfile: getEnv("LIDARR_QUALITY_PROFILE", "Standard"),
			LidarrRootFolders:    getEnvSlice("LIDARR_ROOT_FOLDERS"),
			LidarrDefaultRootFolder: getEnv("LIDARR_DEFAULT_ROOT_FOLDER", ""),
			DownloadMode:         getEnv("DOWNLOAD_MODE", "tracks"),
			DailyCheckCron:       getEnv("DAILY_CHECK_CRON", "0 9 * * *"),
			Timezone:             getEnv("TIMEZONE", "America/Detroit"),
			DBPath:               getEnv("DB_PATH", "/data/groovarr.db"),
			DBSalt:               getEnv("DB_SALT", ""),
		}
	}
	return global
}

// LoadFromDB loads settings from the database and updates the global configuration.
// It should be called after the database has been initialized.
func LoadFromDB() error {
	if global == nil {
		global = Load()
	}

	// Load persistence threshold
	if v, err := store.SettingGet("popularity_threshold"); err == nil && v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			global.PopularityThreshold = i
		}
	}

	// Load download mode
	if v, err := store.SettingGet("download_mode"); err == nil && v != "" {
		if v == "tracks" || v == "album" {
			global.DownloadMode = v
		}
	}

	// Load default root folder
	if v, err := store.SettingGet("lidarr_default_root_folder"); err == nil && v != "" {
		global.LidarrDefaultRootFolder = v
	}

	// Load Lidarr settings
	if v, err := store.SettingGet("lidarr_url"); err == nil && v != "" {
		global.LidarrURL = v
	}
	if v, err := store.SettingGet("lidarr_api_key"); err == nil && v != "" {
		global.LidarrAPIKey = v
	}
	if v, err := store.SettingGet("lidarr_quality_profile"); err == nil && v != "" {
		global.LidarrQualityProfile = v
	}

	// Load Last.fm
	if v, err := store.SettingGet("lastfm_api_key"); err == nil && v != "" {
		global.LastFMAPIKey = v
	}

	// Load schedule
	if v, err := store.SettingGet("daily_check_cron"); err == nil && v != "" {
		global.DailyCheckCron = v
	}
	if v, err := store.SettingGet("timezone"); err == nil && v != "" {
		global.Timezone = v
	}

	// Load Discord settings
	if v, err := store.SettingGet("discord_home_channel"); err == nil && v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			global.DiscordHomeChannel = id
		}
	}
	if v, err := store.SettingGet("discord_allow_users"); err == nil {
		global.DiscordAllowAllUsers = v == "true"
	}
	if v, err := store.SettingGet("discord_auto_thread"); err == nil {
		global.DiscordAutoThread = v == "true"
	}
	if v, err := store.SettingGet("discord_require_mention"); err == nil {
		global.DiscordRequireMention = v == "true"
	}
	// Allowed channels: comma-separated snowflake IDs
	if v, err := store.SettingGet("discord_allowed_channels"); err == nil && v != "" {
		global.DiscordAllowedChans = parseIntSlice(v)
	}
	// Allowed users: comma-separated user IDs or usernames
	if v, err := store.SettingGet("discord_allowed_users"); err == nil && v != "" {
		global.DiscordAllowedUsers = parseStringSlice(v)
	}

	return nil
}

// parseIntSlice parses a comma-separated string of integers.
func parseIntSlice(s string) []int64 {
	parts := strings.Split(s, ",")
	result := make([]int64, 0)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			result = append(result, id)
		}
	}
	return result
}

// parseStringSlice parses a comma-separated string of values.
func parseStringSlice(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Helper functions to get environment variables with type conversion.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
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

func getEnvSlice(key string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	return []string{}
}

// DefaultHTTPClient is a shared HTTP client with timeouts.
var DefaultHTTPClient = &http.Client{Timeout: 30 * time.Second}