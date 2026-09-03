package scheduler

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/core"
	"github.com/groovarr/groovarr/backend/internal/store"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	notifyFunc func(string) // called with a report string when daily check completes
}

func New(notify func(string)) *Scheduler {
	loc, err := time.LoadLocation(config.Load().Timezone)
	if err != nil {
		log.Printf("Invalid timezone '%s', using UTC", config.Load().Timezone)
		loc = time.UTC
	}

	c := cron.New(cron.WithLocation(loc))
	return &Scheduler{cron: c, notifyFunc: notify}
}

func (s *Scheduler) Start() error {
	cfg := config.Load()
	_, err := s.cron.AddFunc(cfg.DailyCheckCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		s.runDailyCheck(ctx)
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	log.Printf("Scheduler started: cron=%s, tz=%s", cfg.DailyCheckCron, cfg.Timezone)
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) runDailyCheck(ctx context.Context) {
	log.Println("Daily check triggered")

	results, err := core.RunDailyCheck("", false, "")
	if err != nil {
		log.Printf("Daily check error: %v", err)
		return
	}

	report := formatCheckReport(results)
	if s.notifyFunc != nil && report != "" {
		s.notifyFunc(report)
	}

	// Also check downloads
	pruneResults, err := core.CheckDownloads()
	if err != nil {
		log.Printf("Download check error: %v", err)
		return
	}

	pruneReport := formatPruneReport(pruneResults)
	if pruneReport != "" && s.notifyFunc != nil {
		s.notifyFunc(pruneReport)
	}
}

// RunNow triggers an immediate check (used by manual trigger endpoints).
func (s *Scheduler) RunNow(ctx context.Context) (string, string, error) {
	results, err := core.RunDailyCheck("", false, "")
	if err != nil {
		return "", "", err
	}
	report := formatCheckReport(results)

	pruneResults, err := core.CheckDownloads()
	pruneReport := ""
	if err == nil {
		pruneReport = formatPruneReport(pruneResults)
	}

	return report, pruneReport, nil
}

// LoadPersistedSettings reloads threshold and mode from DB into config on startup.
func LoadPersistedSettings() {
	if v, _ := store.SettingGet("popularity_threshold"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg := config.Load()
			cfg.PopularityThreshold = i
		}
	}
}

func formatCheckReport(results []core.CheckResult) string {
	if len(results) == 0 {
		return "No artists in watchlist."
	}
	var lines []string
	totalAdded := 0
	for _, r := range results {
		if r.AlbumsAdded > 0 {
			totalAdded += r.AlbumsAdded
			for _, a := range r.AddedAlbums {
				lines = append(lines, a)
			}
		}
	}
	if totalAdded == 0 {
		return "No new popular releases found."
	}
	return "Daily check complete:\n" + join(lines, "\n")
}

func formatPruneReport(results []core.PruneResult) string {
	if len(results) == 0 {
		return ""
	}
	var lines []string
	for _, r := range results {
		if r.Error != "" {
			lines = append(lines, "❌ "+r.ArtistName+" — "+r.AlbumName+": "+r.Error)
		} else {
			lines = append(lines, "✅ "+r.ArtistName+" — "+r.AlbumName+": kept "+strconv.Itoa(r.KeptTracks)+", pruned "+strconv.Itoa(r.PrunedTracks))
		}
	}
	return "Download check + auto-prune:\n" + join(lines, "\n")
}

func join(arr []string, sep string) string {
	if len(arr) == 0 {
		return ""
	}
	result := arr[0]
	for i := 1; i < len(arr); i++ {
		result += sep + arr[i]
	}
	return result
}
