package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/groovarr/groovarr/backend/internal/store"
)

func TestConfigPersistence(t *testing.T) {
	// Create a temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Clean up global config for testing
	Reset()

	// Set environment variables BEFORE loading config
	os.Setenv("POPULARITY_THRESHOLD", "50")
	os.Setenv("DOWNLOAD_MODE", "album")
	os.Setenv("LIDARR_URL", "http://env-lidarr:8686")
	os.Setenv("DAILY_CHECK_CRON", "0 10 * * *")
	os.Setenv("TIMEZONE", "UTC")

	// Initialize database
	if err := store.Init(dbPath); err != nil {
		t.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Load config - this should read from env vars first
	cfg := Load()

	// Verify env var values are loaded
	if cfg.PopularityThreshold != 50 {
		t.Errorf("Expected PopularityThreshold=50 from env, got %d", cfg.PopularityThreshold)
	}
	if cfg.DownloadMode != "album" {
		t.Errorf("Expected DownloadMode=album from env, got %s", cfg.DownloadMode)
	}
	if cfg.LidarrURL != "http://env-lidarr:8686" {
		t.Errorf("Expected LidarrURL from env, got %s", cfg.LidarrURL)
	}
	if cfg.DailyCheckCron != "0 10 * * *" {
		t.Errorf("Expected DailyCheckCron from env, got %s", cfg.DailyCheckCron)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Expected Timezone from env, got %s", cfg.Timezone)
	}

	// Now persist different values to the database
	if err := store.SettingUpdate("popularity_threshold", "75"); err != nil {
		t.Fatalf("Failed to update popularity_threshold: %v", err)
	}
	if err := store.SettingUpdate("download_mode", "tracks"); err != nil {
		t.Fatalf("Failed to update download_mode: %v", err)
	}
	if err := store.SettingUpdate("lidarr_url", "http://db-lidarr:8686"); err != nil {
		t.Fatalf("Failed to update lidarr_url: %v", err)
	}
	if err := store.SettingUpdate("daily_check_cron", "0 12 * * *"); err != nil {
		t.Fatalf("Failed to update daily_check_cron: %v", err)
	}
	if err := store.SettingUpdate("timezone", "America/New_York"); err != nil {
		t.Fatalf("Failed to update timezone: %v", err)
	}

	// Reset global config to simulate restart
	Reset()

	// Load config again - should now read from DB (overriding env vars)
	cfg = Load()

	// Verify DB values take precedence over env vars
	if cfg.PopularityThreshold != 75 {
		t.Errorf("Expected PopularityThreshold=75 from DB, got %d", cfg.PopularityThreshold)
	}
	if cfg.DownloadMode != "tracks" {
		t.Errorf("Expected DownloadMode=tracks from DB, got %s", cfg.DownloadMode)
	}
	if cfg.LidarrURL != "http://db-lidarr:8686" {
		t.Errorf("Expected LidarrURL from DB, got %s", cfg.LidarrURL)
	}
	if cfg.DailyCheckCron != "0 12 * * *" {
		t.Errorf("Expected DailyCheckCron from DB, got %s", cfg.DailyCheckCron)
	}
	if cfg.Timezone != "America/New_York" {
		t.Errorf("Expected Timezone from DB, got %s", cfg.Timezone)
	}
}

func TestConfigFallbackToEnvWhenDBEmpty(t *testing.T) {
	// Create a temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test2.db")

	// Clean up global config for testing
	Reset()

	// Set environment variables
	os.Setenv("POPULARITY_THRESHOLD", "40")
	os.Setenv("DOWNLOAD_MODE", "tracks")

	// Initialize database
	if err := store.Init(dbPath); err != nil {
		t.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Load config - DB is empty, should use env vars
	cfg := Load()

	if cfg.PopularityThreshold != 40 {
		t.Errorf("Expected PopularityThreshold=40 from env (DB empty), got %d", cfg.PopularityThreshold)
	}
	if cfg.DownloadMode != "tracks" {
		t.Errorf("Expected DownloadMode=tracks from env (DB empty), got %s", cfg.DownloadMode)
	}
}

func TestConfigDefaultsWhenBothEmpty(t *testing.T) {
	// Create a temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test3.db")

	// Clean up global config for testing
	Reset()

	// Unset environment variables
	os.Unsetenv("POPULARITY_THRESHOLD")
	os.Unsetenv("DOWNLOAD_MODE")

	// Initialize database
	if err := store.Init(dbPath); err != nil {
		t.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Load config - both empty, should use defaults
	cfg := Load()

	if cfg.PopularityThreshold != 30 {
		t.Errorf("Expected PopularityThreshold=30 (default), got %d", cfg.PopularityThreshold)
	}
	if cfg.DownloadMode != "tracks" {
		t.Errorf("Expected DownloadMode=tracks (default), got %s", cfg.DownloadMode)
	}
}

func TestSettingsAPIPersistence(t *testing.T) {
	// Create a temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test4.db")

	// Clean up global config for testing
	Reset()

	// Initialize database
	if err := store.Init(dbPath); err != nil {
		t.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Simulate SettingsHandler POST - update a setting
	if err := store.SettingUpdate("popularity_threshold", "100"); err != nil {
		t.Fatalf("Failed to update setting: %v", err)
	}

	// Verify we can read it back
	val, err := store.SettingGet("popularity_threshold")
	if err != nil {
		t.Fatalf("Failed to get setting: %v", err)
	}
	if val != "100" {
		t.Errorf("Expected '100', got '%s'", val)
	}

	// Reset config and reload
	Reset()
	cfg := Load()

	if cfg.PopularityThreshold != 100 {
		t.Errorf("Expected PopularityThreshold=100 after API update, got %d", cfg.PopularityThreshold)
	}
}