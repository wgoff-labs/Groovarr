package config

import (
	"strings"
	"unicode/utf8"
)

// SanitizeError strips potentially sensitive information from error messages
// before they are returned to clients. This prevents leakage of API keys, tokens,
// database paths, and other secrets embedded in error responses.
func SanitizeError(err string) string {
	if err == "" {
		return "an error occurred"
	}

	result := err

	// Remove DB path references (e.g., "./groovarr.db", "/path/to/db")
	result = dbPathRegexReplace(result)

	// Mask API keys and tokens — replace known patterns with placeholder
	result = maskAPIKeys(result)

	// Remove raw SQL/error strings that might contain sensitive data
	result = sanitizeSQLReferences(result)

	// Strip secret-setting key names from error messages (e.g., lidarr_api_key=)
	result = sanitizeSecretKeys(result)

	// Truncate overly long error messages
	if utf8.RuneCountInString(result) > 200 {
		result = result[:197] + "..."
	}

	return result
}

// dbPathRegexReplace removes references to database file paths
func dbPathRegexReplace(s string) string {
	// Apply simple string replacements for common DB path patterns
	s = strings.ReplaceAll(s, "./groovarr.db", "database")
	s = strings.ReplaceAll(s, "groovarr.db", "[db]")
	return s
}

// maskAPIKeys replaces known secret patterns with redacted placeholders
func maskAPIKeys(s string) string {
	// Lidarr API key patterns
	s = strings.ReplaceAll(s, "LidarrAPIKey", "[redacted]")
	s = strings.ReplaceAll(s, "Lidarr API key", "[redacted]")

	// Last.fm API key patterns
	s = strings.ReplaceAll(s, "LastFMAPIKey", "[redacted]")
	s = strings.ReplaceAll(s, "Last.fm API key", "[redacted]")

	// Discord token patterns
	s = strings.ReplaceAll(s, "DISCORD_BOT_TOKEN", "[redacted]")
	s = strings.ReplaceAll(s, "Discord Bot Token", "[redacted]")
	s = strings.ReplaceAll(s, "discord token", "[redacted]")
	s = strings.ReplaceAll(s, "DiscordToken", "[redacted]")

	// DB salt patterns
	s = strings.ReplaceAll(s, "DB_SALT", "[redacted]")
	s = strings.ReplaceAll(s, "DB Salt", "[redacted]")

	return s
}

// sanitizeSQLReferences removes SQL-related content that might leak data
func sanitizeSQLReferences(s string) string {
	// Common patterns that might leak data
	s = strings.ReplaceAll(s, "INSERT", "[redacted]")
	s = strings.ReplaceAll(s, "UPDATE", "[redacted]")
	s = strings.ReplaceAll(s, "DELETE", "[redacted]")
	s = strings.ReplaceAll(s, "SELECT", "[redacted]")
	return s
}

// sanitizeSecretKeys strips known secret-setting key names from error messages
func sanitizeSecretKeys(s string) string {
	// These are the setting keys that should never appear in error output
	secrets := []string{
		"lidarr_api_key=",
		"lidarr_api_key",
		"discord_token=",
		"discord_token",
		"lastfm_api_key=",
		"lastfm_api_key",
		"auth_password=",
		"auth_password",
	}
	for _, key := range secrets {
		s = strings.ReplaceAll(s, key, "[redacted]=")
	}
	return s
}