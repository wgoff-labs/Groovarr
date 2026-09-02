package core

import (
	"log"
	"time"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// PopularityRefresher walks the artist table in the background and refreshes
// popularity caches that are older than CacheTTL. It respects Last.fm's rate
// limit by sleeping 12 seconds between artists (5 req/min) — well under the
// 5 req/sec allowed but conservative enough to avoid any chance of throttling.
//
// Why this exists:
//   - First-time artists need a Last.fm fetch on their first /api/check.
//   - Caching means the second check is free.
//   - This background goroutine spreads refreshes across the week so the cache
//     stays warm and no artist ever has to wait for a Last.fm round-trip.
type PopularityRefresher struct {
	stop chan struct{}
}

// NewPopularityRefresher creates a refresher but does not start it.
func NewPopularityRefresher() *PopularityRefresher {
	return &PopularityRefresher{stop: make(chan struct{})}
}

// Start runs the background refresh loop. Safe to call once at startup.
func (p *PopularityRefresher) Start() {
	go p.loop()
}

// Stop halts the background loop.
func (p *PopularityRefresher) Stop() {
	close(p.stop)
}

func (p *PopularityRefresher) loop() {
	// Wait a bit on startup so we don't compete with /api/check on first deploy.
	time.Sleep(2 * time.Minute)
	log.Printf("[popularity] background refresher started (TTL=%s)", CacheTTL)

	for {
		select {
		case <-p.stop:
			return
		default:
		}

		// Pull a small batch of stale artists. Limit to 1 to spread load
		// across the day: with 5 req/min, 60 minutes = 300 artists/day,
		// which covers a watchlist of thousands of artists in under a week.
		ids, names, err := store.StaleArtistIDs(CacheTTL, 1)
		if err != nil {
			log.Printf("[popularity] failed to list stale artists: %v", err)
			time.Sleep(5 * time.Minute)
			continue
		}

		if len(ids) == 0 {
			// Everything fresh: sleep for an hour and check again.
			time.Sleep(1 * time.Hour)
			continue
		}

		for i, id := range ids {
			select {
			case <-p.stop:
				return
			default:
			}

			// Resolve Deezer ID (cached, no API call) for the gap-filler.
			a, _ := store.ArtistGetByID(id)
			deezerID := ""
			if a != nil && a.DeezID != "" {
				deezerID = a.DeezID
			}

			// Fetch and discard the result — it just refreshes the cache.
			GetArtistTrackScores(id, names[i], deezerID)
			log.Printf("[popularity] refreshed '%s' (id=%d)", names[i], id)

			// Last.fm rate limit: 5 req/sec allowed, we use 5 req/min to be safe.
			time.Sleep(12 * time.Second)
		}
	}
}
