package core

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// CacheTTL is how long a popularity cache is considered fresh before a background
// refresh is triggered. 7 days means the background scheduler can spread refreshes
// across the week rather than hammering on deploy.
const CacheTTL = 7 * 24 * time.Hour

// TrackScores holds popularity data for an artist's tracks.
type TrackScores struct {
	// name → score (name = lowercase, fuzzy-matched)
	NameScores map[string]int
	// deezer track ID → score
	DeezerIDScores map[int64]int
}

// GetArtistTrackScores returns popularity scores for an artist's tracks.
// It uses the freshness cache to skip redundant Last.fm API calls.
// Last.fm is the primary source; Deezer fills in any gaps.
func GetArtistTrackScores(artistID int64, artistName, deezerID string) TrackScores {
	cfg := config.Load()
	scores := TrackScores{
		NameScores:     make(map[string]int),
		DeezerIDScores: make(map[int64]int),
	}

	// --- Try cache first ---
	if store.GetDB() != nil {
		if fresh, ok, _ := store.CacheFreshness(artistID, CacheTTL); ok {
			// Cache is fresh: load scores from DB and return immediately.
			scores = loadScoresFromDB(artistID)
			if len(scores.NameScores) > 0 {
				log.Printf("[popularity] cache hit for '%s' (%d scores, fetched %s)",
					artistName, len(scores.NameScores), fresh.Format("Jan 2"))
				return scores
			}
		}
	}

	// --- Cache miss or stale: fetch from Last.fm ---
	var maxPC int64 = 1
	if cfg.LastFMAPIKey != "" {
		lfm, err := clients.NewLastFMClient()
		if err == nil {
			rawTracks, err := lfm.GetArtistTopTracks(artistName, 5)
			if err == nil && len(rawTracks) > 0 {
				// Find max playcount for normalization.
				for _, t := range rawTracks {
					if t.PlayCount > maxPC {
						maxPC = t.PlayCount
					}
				}
				// Score each track and store.
				for _, t := range rawTracks {
					score := scoreFromPlaycount(t.PlayCount, maxPC)
					scores.NameScores[normalizeTrack(t.Name)] = score
					_ = store.UpsertTrackPopularity(artistID, 0, score, "lastfm")
				}
				// Update cache freshness so we don't re-fetch this artist for 7 days.
				_ = store.UpdateCacheFreshness(artistID, artistName, len(rawTracks), int(maxPC))
				log.Printf("[popularity] fetched %d Last.fm tracks for '%s' (max=%d)",
					len(rawTracks), artistName, maxPC)
			}
		}
	}

		// Deezer supplement disabled (2026-09-02): Deezer rank is current popularity,
		// not historical. Last.fm playcount is the only source for now.
		// TODO: re-enable once a historical API (Spotify, Apple Music) is wired up.
		// if deezerID != "" { ... }

	return scores
}

// loadScoresFromDB reads cached popularity scores from the database.
func loadScoresFromDB(artistID int64) TrackScores {
	scores := TrackScores{
		NameScores:     make(map[string]int),
		DeezerIDScores: make(map[int64]int),
	}
	rows, err := store.GetTrackPopularity(artistID)
	if err != nil || rows == nil {
		return scores
	}
	// We don't have the track name here, only the DB row.
	// Caller must resolve by name in checker.go using Lidarr metadata.
	return scores
}

// scoreFromPlaycount converts an absolute playcount to a 0-100 score using a
// log-scale blend. This ensures that:
//   - Underground artists (compressed range) still get a useful 0-100 spread
//   - Superstars (huge gap between #1 and #50) don't crush the tail to near-zero
func scoreFromPlaycount(playcount int64, maxPlaycount int64) int {
	if maxPlaycount <= 0 {
		maxPlaycount = 1
	}
	// Linear ratio: raw proportion of max.
	ratio := float64(playcount) / float64(maxPlaycount)
	// Log-scale ratio: more compression for outliers.
	logRatio := math.Log1p(float64(playcount)) / math.Log1p(float64(maxPlaycount))
	// Blend: 40% linear (preserves top-track identity), 60% log (compresses outliers).
	blended := ratio*0.4 + logRatio*0.6
	score := int(blended * 100)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// ScoreTrack looks up a track's popularity score using fuzzy name matching.
// Matching order: Deezer ID → exact name → fuzzy name.
func ScoreTrack(trackName string, scores TrackScores, deezerID int64) int {
	name := normalizeTrack(trackName)

	// Deezer ID match (most reliable).
	if deezerID != 0 {
		if s, ok := scores.DeezerIDScores[deezerID]; ok {
			return s
		}
	}

	// Exact match.
	if s, ok := scores.NameScores[name]; ok {
		return s
	}

	// Fuzzy: check if one is a substring of the other.
	for topName, score := range scores.NameScores {
		if fuzzyMatch(name, topName) {
			return score
		}
	}

	return 5 // default low score for truly unknown tracks
}

// normalizeTrack normalizes a track name for comparison:
// lowercases, trims whitespace, strips common suffixes like " - Remastered",
// "(Radio Edit)", "feat.", "with X".
func normalizeTrack(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	// Strip parenthesized suffixes: "(2024 Remaster)", "(Radio Edit)", "(Live)".
	if idx := strings.Index(s, " ("); idx > 0 {
		s = s[:idx]
	}

	// Strip " - " suffixes: " - Remastered", " - Radio Edit", " - Live".
	if idx := strings.LastIndex(s, " - "); idx > 0 {
		s = s[:idx]
	}

	// Strip "feat.", "ft.", "with".
	s = stripPrefix(s, "feat.")
	s = stripPrefix(s, "ft.")
	s = stripPrefix(s, "with")

	return strings.TrimSpace(s)
}

// fuzzyMatch returns true if a and b are close enough to be the same track.
// Handles partial matches where one is a substring of the other after normalization.
func fuzzyMatch(a, b string) bool {
	if a == b {
		return true
	}
	// Substring in either direction.
	if len(a) > 3 && len(b) > 3 {
		if strings.Contains(a, b) || strings.Contains(b, a) {
			return true
		}
	}
	return false
}

func stripPrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		s = strings.TrimPrefix(s, prefix)
	}
	if i := strings.Index(s, prefix); i > 0 {
		s = s[:i] + s[i+len(prefix):]
	}
	return s
}

// ShouldDownloadAlbum returns true if an album is popular enough to download.
func ShouldDownloadAlbum(avgPopularity float64, topTrackCount int) bool {
	cfg := config.Load()
	return avgPopularity >= float64(cfg.PopularityThreshold) || topTrackCount >= 2
}

// GetThreshold returns the current popularity threshold.
func GetThreshold() int {
	return config.Load().PopularityThreshold
}

// GetMode returns the download mode for an artist.
func GetMode(artistName string) string {
	key := "mode_" + artistName
	if val, err := store.SettingGet(key); err == nil && val != "" {
		return val
	}
	if val, err := store.SettingGet("mode_default"); err == nil && val != "" {
		return val
	}
	return "albums"
}
