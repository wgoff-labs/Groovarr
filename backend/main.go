package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/groovarr/groovarr/backend/internal/api"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/connections"
	"github.com/groovarr/groovarr/backend/internal/core"
	"github.com/groovarr/groovarr/backend/internal/discord"
	"github.com/groovarr/groovarr/backend/internal/frontend"
	"github.com/groovarr/groovarr/backend/internal/scheduler"
	"github.com/groovarr/groovarr/backend/internal/store"
	"github.com/groovarr/groovarr/backend/internal/clients"
)

// Version and BuildNumber are set at build time via -ldflags.
// BuildNumber is auto-incremented every commit (git rev-list --count HEAD).
var Version = "dev"
var BuildNumber = "0"

func main() {
	log.SetPrefix("[groovarr] ")
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg := config.Load()
	log.Printf("Starting Groovarr (config: %+v)", cfg)

	// Initialize database
	if err := store.Init(cfg.DBPath); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Reconcile environment variables to database on first run
	config.ReconcileEnvToDB()

	// Load persisted settings (Lidarr URL/key, Discord tokens, etc.)
	// into the global config so that calls to config.Load() reflect user-saved
	// values rather than just the environment-variable defaults.
	config.LoadFromDB()

	// On a clean database initialization, fetch the first available Lidarr quality
	// profile and root folder from the Lidarr API and write them to the settings
	// table so bootstrap works without manual config.
	if store.GetDB() != nil {
		qpVal, qpErr := store.SettingGet("lidarr_quality_profile")
		if qpErr != nil || qpVal == "" {
			if err := fetchLidarrQualityProfileToDB(); err != nil {
				log.Printf("[config] warning: failed to fetch Lidarr quality profile: %v", err)
			}
		}
		rfVal, rfErr := store.SettingGet("lidarr_default_root_folder")
		if rfErr != nil || rfVal == "" {
			if err := fetchLidarrDefaultRootFolderToDB(); err != nil {
				log.Printf("[config] warning: failed to fetch Lidarr default root folder: %v", err)
			}
		}
	}

	// Create HTTP mux
	mux := http.NewServeMux()
	mux.HandleFunc("/api/artists", api.ArtistHandler)
	mux.HandleFunc("/api/artists/import", api.ArtistImportHandler)
	mux.HandleFunc("/api/artists/import/bulk", api.ArtistImportBulkHandler)
	mux.HandleFunc("/api/artist/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/manage") {
			api.ArtistManageHandler(w, r)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/track/") && strings.HasSuffix(r.URL.Path, "/state") {
			api.TrackStateHandler(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/folders", api.FoldersHandler)
	mux.HandleFunc("/api/profiles", api.ProfilesHandler)
	mux.HandleFunc("/api/check", api.CheckHandler)
	mux.HandleFunc("/api/check/status", api.CheckStatusHandler)
	mux.HandleFunc("/api/scan", api.ScanHandler)
	mux.HandleFunc("/api/prune", api.PruneHandler)
	mux.HandleFunc("/api/settings", api.SettingsHandler)
	mux.HandleFunc("/api/keep", api.KeepHandler)
	mux.HandleFunc("/api/downloads", api.DownloadStatusHandler)
	mux.HandleFunc("/api/connections", connections.ConnectionsHandler)
	mux.HandleFunc("/api/connections/logs", connections.LogsHandler)
	mux.HandleFunc("/api/hit-fallen", api.HitFallenHandler)
	mux.HandleFunc("/api/status", api.StatusHandler)
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": Version, "build": BuildNumber})
	})
	// Wrap the API mux with auth middleware (applies to all /api/ routes)
	authMux := api.AuthMiddleware(mux)

	// Go's http.ServeMux uses longest-prefix match, so "/" only matches "/" not "/artists".
	// We need to use a custom handler that routes API vs frontend.
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			authMux.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/_debug/") {
			switch r.URL.Path {
			case "/_debug/node":
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(frontend.DebugNodeStatus())
				return
			}
		}
		frontend.NewHandler()(w, r)
	})

	handler := corsMiddleware(mainHandler)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	}

	// Initialise external connections (Lidarr, Discord, LastFM) from saved credentials.
	// This also starts the Discord bot if a token is in the DB.
	connections.Init()

	// Get a reference to the Discord bot that may have been started by connections.Init().
	// If no token was in the DB the bot will be nil — that's fine.
	discordBot := discord.GetBot()

	// Create scheduler — uses Discord bot for reports if available
	sch := scheduler.New(func(report string) {
		if discordBot != nil && cfg.DiscordHomeChannel != 0 {
			if err := discordBot.SendReport(report); err != nil {
				log.Printf("Failed to send scheduled report to Discord: %v", err)
			}
		} else {
			log.Printf("Scheduled report: %s", report)
		}
	})

	// Start scheduler
	if err := sch.Start(); err != nil {
		log.Fatalf("Scheduler start failed: %v", err)
	}

	// Start background popularity refresher (keeps Last.fm cache warm).
	refresher := core.NewPopularityRefresher()
	refresher.Start()

	// Start HTTP server
	go func() {
		log.Printf("HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sch.Stop()
	if discordBot != nil {
		discordBot.Stop()
	}
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("Groovarr stopped")
}

// fetchLidarrQualityProfileToDB fetches the first available Lidarr quality profile
// from the Lidarr API and writes its ID to the settings table.
func fetchLidarrQualityProfileToDB() error {
	url, err := store.SettingGet("lidarr_url")
	if err != nil || url == "" {
		return fmt.Errorf("lidarr URL not configured")
	}
	key, err := store.SettingGet("lidarr_api_key")
	if err != nil || key == "" {
		return fmt.Errorf("lidarr API key not configured")
	}

	c, err := clients.NewLidarrClientWith(url, key)
	if err != nil {
		return fmt.Errorf("failed to create Lidarr client: %w", err)
	}

	profiles, err := c.GetQualityProfiles()
	if err != nil {
		return fmt.Errorf("failed to fetch quality profiles from Lidarr: %w", err)
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no quality profiles found in Lidarr")
	}

	return store.SettingUpdate("lidarr_quality_profile", fmt.Sprintf("%d", profiles[0].ID))
}

// fetchLidarrDefaultRootFolderToDB fetches the first available Lidarr root folder
// from the Lidarr API and writes its path to the settings table.
func fetchLidarrDefaultRootFolderToDB() error {
	url, err := store.SettingGet("lidarr_url")
	if err != nil || url == "" {
		return fmt.Errorf("lidarr URL not configured")
	}
	key, err := store.SettingGet("lidarr_api_key")
	if err != nil || key == "" {
		return fmt.Errorf("lidarr API key not configured")
	}

	c, err := clients.NewLidarrClientWith(url, key)
	if err != nil {
		return fmt.Errorf("failed to create Lidarr client: %w", err)
	}

	folders, err := c.GetRootFolders()
	if err != nil {
		return fmt.Errorf("failed to fetch root folders from Lidarr: %w", err)
	}
	if len(folders) == 0 {
		return fmt.Errorf("no root folders found in Lidarr")
	}

	return store.SettingUpdate("lidarr_default_root_folder", folders[0].Path)
}

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
