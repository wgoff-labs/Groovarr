package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/groovarr/groovarr/backend/internal/api"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/discord"
	"github.com/groovarr/groovarr/backend/internal/scheduler"
	"github.com/groovarr/groovarr/backend/internal/store"
)

func main() {
	log.SetPrefix("[groovarr] ")
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg := config.Load()
	log.Printf("Starting Groovarr (config: %+v)", cfg)

	// Initialize database
	if err := store.Init(cfg); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Load persisted settings into config
	scheduler.LoadPersistedSettings()

	// Create HTTP mux
	mux := http.NewServeMux()
	mux.HandleFunc("/api/artists", api.ArtistHandler)
	mux.HandleFunc("/api/status", api.StatusHandler)
	mux.HandleFunc("/api/check", api.CheckHandler)
	mux.HandleFunc("/api/scan", api.ScanHandler)
	mux.HandleFunc("/api/prune", api.PruneHandler)
	mux.HandleFunc("/api/settings", api.SettingsHandler)
	mux.HandleFunc("/api/keep", api.KeepHandler)
	mux.HandleFunc("/api/downloads", api.DownloadStatusHandler)

	// Serve static files from frontend build (embedded or mounted)
	// In dev, we proxy to Vite dev server; in prod, we serve from ./frontend/dist
	// For now, we just serve a placeholder.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Groovarr backend is running. Frontend not served yet."))
	})

	// CORS wrapper for dev
	handler := corsMiddleware(mux)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	}

	// Create Discord bot (optional — app runs without it)
	var discordBot *discord.Bot
	if cfg.DiscordToken != "" {
		var berr error
		discordBot, berr = discord.New(cfg.DiscordToken, func(report string) {
			if cfg.ReportChannelID != 0 {
				if bot := discord.GetBot(); bot != nil {
					if err := bot.SendReport(uint64(cfg.ReportChannelID), report); err != nil {
						log.Printf("Failed to send report to Discord: %v", err)
					}
				}
			} else {
				log.Printf("Report (no channel set): %s", report)
			}
		})
		if berr != nil {
			log.Printf("Discord bot init failed (continuing without): %v", berr)
		} else {
			discord.SetGlobalBot(discordBot)
			if err := discordBot.Start(); err != nil {
				log.Printf("Discord bot start failed (continuing without): %v", err)
			} else {
				log.Println("Discord bot connected")
			}
		}
	} else {
		log.Println("Discord token not set — running without bot")
	}

	// Create scheduler
	sch := scheduler.New(func(report string) {
		if discordBot != nil && cfg.ReportChannelID != 0 {
			if err := discordBot.SendReport(uint64(cfg.ReportChannelID), report); err != nil {
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