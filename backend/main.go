package main

import (
	"context"
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
	"github.com/groovarr/groovarr/backend/internal/discord"
	"github.com/groovarr/groovarr/backend/internal/frontend"
	"github.com/groovarr/groovarr/backend/internal/scheduler"
	"github.com/groovarr/groovarr/backend/internal/store"
)

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

	// Load persisted settings into config
	scheduler.LoadPersistedSettings()

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
	mux.HandleFunc("/api/scan", api.ScanHandler)
	mux.HandleFunc("/api/prune", api.PruneHandler)
	mux.HandleFunc("/api/settings", api.SettingsHandler)
	mux.HandleFunc("/api/keep", api.KeepHandler)
	mux.HandleFunc("/api/downloads", api.DownloadStatusHandler)
	mux.HandleFunc("/api/connections", connections.ConnectionsHandler)
	mux.HandleFunc("/api/connections/logs", connections.LogsHandler)
	mux.HandleFunc("/api/hit-fallen", api.HitFallenHandler)

	// Serve embedded frontend (handles all non-API routes including SPA routing)
	// Go's http.ServeMux uses longest-prefix match, so "/" only matches "/" not "/artists".
	// We need to use a custom handler that routes API vs frontend.
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			mux.ServeHTTP(w, r)
			return
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
