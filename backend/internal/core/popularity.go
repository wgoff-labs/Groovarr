package core

import (
	"log"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// TrackScores holds popularity scores for an artist's tracks.
// Key = lowercase track name. Special key "__deezer_ids__" holds Deezer ID → score.
type TrackScores struct {
	// name → score
	NameScores map[string]int
	// deezer track ID → score
	DeezerIDScores map[int64]int
}

// GetArtistTrackScores fetches popularity data for an artist.
// Last.fm is primary; Deezer fills gaps.
func GetArtistTrackScores(artistName, deezerID string) TrackScores {
	scores := TrackScores{
		NameScores:     make(map[string]int),
		DeezerIDScores: make(map[int64]int),
	}

	cfg := config.Load()

	// Last.fm primary
	if cfg.LastFMAPIKey != "" {
		lfm, err := clients.NewLastFMClient()
		if err == nil {
			lfmScores, err := lfm.GetArtistTopTracksScored(artistName)
			if err == nil && len(lfmScores) > 0 {
				scores.NameScores = lfmScores
				log.Printf("Last.fm: %d scores for '%s'", len(lfmScores), artistName)
			}
		}
	}

	// Deezer supplement
	if deezerID != "" {
		deezer := clients.NewDeezerClient()
		topTracks, err := deezer.GetArtistTopTracks(deezerID)
		if err == nil && len(topTracks) > 0 {
			maxRank := int64(0)
			for _, t := range topTracks {
				if t.Rank > maxRank {
					maxRank = t.Rank
				}
			}
			if maxRank == 0 {
				maxRank = 1
			}
			for i, t := range topTracks {
				score := max(10, min(100, int(float64(t.Rank)/float64(maxRank)*100)))
				if score == 0 {
					score = max(50, 100-min(50, int(float64(i)/float64(len(topTracks))*50)))
				}
				name := normalize(t.Title)
				if _, exists := scores.NameScores[name]; !exists {
					scores.NameScores[name] = score
				}
				scores.DeezerIDScores[t.ID] = score
			}
			log.Printf("Deezer: supplemented scores for '%s'", artistName)
		}
	}

	return scores
}

// ScoreTrack scores a single track using the given scores.
func ScoreTrack(trackName string, scores TrackScores, deezerID int64) int {
	name := normalize(trackName)

	// Deezer ID match
	if deezerID != 0 {
		if s, ok := scores.DeezerIDScores[deezerID]; ok {
			return s
		}
	}

	// Exact name match
	if s, ok := scores.NameScores[name]; ok {
		return s
	}

	// Fuzzy name match
	for topName, score := range scores.NameScores {
		if contains(name, topName) || contains(topName, name) {
			return score
		}
	}

	return 10 // default for unknown tracks
}

// ShouldDownloadAlbum returns true if an album has enough popular tracks.
func ShouldDownloadAlbum(avgPopularity float64, topTrackCount int) bool {
	cfg := config.Load()
	threshold := cfg.PopularityThreshold
	return avgPopularity >= float64(threshold) || topTrackCount >= 2
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return s
}

func contains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		(len(haystack) >= len(needle)) &&
		(strings.Contains(haystack, needle) || strings.Contains(needle, haystack))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetThreshold returns the current popularity threshold (0–100).
func GetThreshold() int {
	cfg := config.Load()
	return cfg.PopularityThreshold
}

// GetMode returns the download mode ("tracks" or "albums") for an artist,
// falling back to the global default.
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
