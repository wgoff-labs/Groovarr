package core

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// CheckResult is the result of checking one artist.
type CheckResult struct {
	ArtistName          string   `json:"artist_name"`
	NewAlbumsFound      int      `json:"new_albums_found"`
	AlbumsAdded         int      `json:"albums_added"`
	TracksAdded         int      `json:"tracks_added"`
	TracksSkipped       int      `json:"tracks_skipped"`
	AlbumsSkipped       int      `json:"albums_skipped"`
	Errors              []string `json:"errors"`
	AddedAlbums         []string `json:"added_albums"`
	SkippedAlbums       []string `json:"skipped_albums"`
	HitsKept            int      `json:"hits_kept"`
	HitsFallen          int      `json:"hits_fallen"`
	TracksPruned        int      `json:"tracks_pruned"`
}

// RunDailyCheck runs the daily popularity check for all (or one) watched artists.
func RunDailyCheck(artistFilter string, fullScan bool) ([]CheckResult, error) {
	var artists []*store.Artist
	if artistFilter != "" {
		a, err := store.ArtistGet(artistFilter)
		if err != nil || a == nil {
			return nil, fmt.Errorf("artist '%s' not found", artistFilter)
		}
		artists = []*store.Artist{a}
	} else {
		var err error
		artists, err = store.ArtistList()
		if err != nil {
			return nil, err
		}
	}

	if len(artists) == 0 {
		return nil, nil
	}

	cfg := config.Load() // always reload so DB changes (settings API) take effect without restart
	deezer := clients.NewDeezerClient()
	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return nil, fmt.Errorf("lidarr: %w", err)
	}

	var results []CheckResult

	for _, artist := range artists {
		result := CheckResult{ArtistName: artist.Name}
		log.Printf("Checking artist: %s", artist.Name)

		// Resolve Deezer ID
		deezerID := artist.DeezID
		if deezerID == "" {
			found, err := deezer.SearchArtist(artist.Name)
			if err != nil || found == nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Not found on Deezer: %v", err))
				results = append(results, result)
				continue
			}
			idStr := fmt.Sprintf("%d", found.ID)
			deezerID = idStr
			store.ArtistUpdateDeezerID(artist.Name, idStr)
		}

		// Fetch Last.fm popularity data (primary) before processing albums.
		// Deezer supplements Last.fm gaps via deezerAlbum.TrackPopularities.
		lastfmScores := GetArtistTrackScores(artist.Name, deezerID)

		// Get releases
		var albums []clients.AlbumInfo
		var err error
		if fullScan {
			albums, err = deezer.GetTopAlbums(deezerID)
		} else {
			albums, err = deezer.GetNewReleases(deezerID, 90)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Music API error: %v", err))
			results = append(results, result)
			continue
		}

		popular := filterPopularAlbums(albums)
		result.NewAlbumsFound = len(popular)

		// Ensure artist in Lidarr
		lidarrArtistID := artist.LidarrID
		if lidarrArtistID == nil || *lidarrArtistID == 0 {
			added, err := ensureLidarrArtist(artist, lidarr)
			if err != nil || added == nil {
				result.Errors = append(result.Errors, "Could not add artist to Lidarr")
				results = append(results, result)
				continue
			}
			lidarrArtistID = &added.ID
			store.ArtistUpdateLidarrID(artist.Name, added.ID)
		}

		// Fetch Lidarr albums
		var lidarrAlbums []clients.LidarrAlbum
		if lidarrArtistID != nil {
			lidarrAlbums, _ = lidarr.GetArtistAlbums(*lidarrArtistID)
			if len(lidarrAlbums) == 0 {
				log.Printf("No Lidarr albums for artist %s, waiting for refresh...", artist.Name)
				waitForLidarrAlbums(lidarr, *lidarrArtistID, &lidarrAlbums)
			}
		}

		// 3-state model: Only run track-level processing if in "tracks" mode.
		// In "album" mode, behavior remains: monitor + search the whole album.
		downloadMode := cfg.DownloadMode
		if artistMode, err := store.SettingGet("mode_" + artist.Name); err == nil && artistMode != "" {
			downloadMode = artistMode
		}

		for _, album := range popular {
			matched := matchAlbum(album.Name, lidarrAlbums)
			if matched == nil {
				store.CheckLogInsert(artist.ID, album.Name, &album.DeezerURL, int(album.AvgPopularity), false)
				result.SkippedAlbums = append(result.SkippedAlbums, fmt.Sprintf("%s (not in Lidarr)", album.Name))
				continue
			}

			if downloadMode == "tracks" {
				// Per-track 3-state model.
				hitsKept, hitsFallen, pruned := processTracksThreeState(
					artist, matched, album, lidarr, cfg.PopularityThreshold, lastfmScores, &result,
				)
				result.HitsKept += hitsKept
				result.HitsFallen += hitsFallen
				result.TracksPruned += pruned
			} else {
				// Album mode: existing behavior — monitor + search the whole album.
				if err := lidarr.MonitorAndSearchAlbum(matched.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to search %s: %v", album.Name, err))
					continue
				}
				// Note: AlbumStatusSet function doesn't exist in store, skipping for now
				store.CheckLogInsert(artist.ID, album.Name, &album.DeezerURL, int(album.AvgPopularity), true)
				result.AddedAlbums = append(result.AddedAlbums, fmt.Sprintf("%s (avg pop: %.1f, type: %s)", album.Name, album.AvgPopularity, album.AlbumType))
				result.AlbumsAdded++
			}
		}

		store.ArtistMarkChecked(artist.ID)
		results = append(results, result)
	}

	return results, nil
}

// processTracksThreeState applies the per-track 3-state (keep / hit / not_keep) model
// for one album. Returns counters for hits kept, hits fallen (logged), and tracks pruned.
//
// State semantics (per plan §4):
//   - "keep"      → ensure downloaded (search if missing); never prune
//   - "hit"       → if score >= threshold: do nothing; if score < threshold:
//                   log to hit_fallen_log, leave UI state as "hit", do NOT download,
//                   do NOT auto-prune (user must act)
//   - "not_keep"  → if downloaded: unmonitor in Lidarr (files remain); never re-download
//   - "" (none)   → apply automation: keep if score >= threshold; mark for pruning if
//                   downloaded AND score < threshold
func processTracksThreeState(
	artist *store.Artist,
	album *clients.LidarrAlbum,
	deezerAlbum clients.AlbumInfo,
	lidarr *clients.LidarrClient,
	threshold int,
	lastfmScores TrackScores,
	result *CheckResult,
) (hitsKept, hitsFallen, pruned int) {
	tracks, err := lidarr.GetAlbumTracks(album.ID)
	if err != nil || len(tracks) == 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("No tracks for %s: %v", deezerAlbum.Name, err))
		return
	}

	// Map of track name (lower) → score from Deezer album tracks (fallback / supplement).
	deezerScoreByName := make(map[string]int)
	for _, tp := range deezerAlbum.TrackPopularities {
		deezerScoreByName[strings.ToLower(strings.TrimSpace(tp.Name))] = tp.Popularity
	}

	for _, t := range tracks {
		key := strings.ToLower(strings.TrimSpace(t.Title))
		// Score lookup: Last.fm (primary) → Deezer fallback → default 10.
		score := lastfmScores.NameScores[key]
		if score == 0 {
			if score = deezerScoreByName[key]; score == 0 {
				score = 10
			}
		}

		// Update popularity cache for any future queries.
		_ = store.UpsertTrackPopularity(artist.ID, t.ID, score, "lastfm")

		state, _ := store.GetTrackPreference(artist.ID, t.ID)

		switch state {
		case "keep":
			if !t.HasFile {
				// Search for this specific track in Lidarr.
				if err := lidarr.SearchTrack(t.ID); err == nil {
					result.TracksAdded++
				}
			} else {
				result.TracksSkipped++
			}

		case "hit":
			if score >= threshold {
				hitsKept++
			} else {
				// Score fell below threshold — log it but DO NOT change state
				// and DO NOT auto-prune. User decides.
				_ = store.HitFallenLogInsert(artist.ID, t.ID, t.Title, score)
				hitsFallen++
			}

		case "not_keep":
			if t.HasFile {
				// Conservative: unmonitor in Lidarr, leave files on disk.
				if err := lidarr.UnmonitorTrack(t.ID); err == nil {
					_ = store.TrackActionLog(artist.ID, t.ID, "unmonitored", "")
					pruned++
				}
			}

		default:
			// No preference — apply automation.
			if score >= threshold {
				if !t.HasFile {
					// Trigger search; do not set explicit preference.
					_ = lidarr.SearchTrack(t.ID)
					result.TracksAdded++
				} else {
					result.TracksSkipped++
				}
			} else {
				if t.HasFile {
					// Mark for pruning: unmonitor + log.
					if err := lidarr.UnmonitorTrack(t.ID); err == nil {
						_ = store.TrackActionLog(artist.ID, t.ID, "unmonitored", "")
						_ = store.PruningLogInsert(artist.ID, deezerAlbum.Name, "unmonitored", "", 1)
						pruned++
					}
				}
			}
		}
	}

	store.CheckLogInsert(artist.ID, deezerAlbum.Name, &deezerAlbum.DeezerURL, int(deezerAlbum.AvgPopularity), true)
	result.AlbumsAdded++
	return
}

func filterPopularAlbums(albums []clients.AlbumInfo) []clients.AlbumInfo {
	var out []clients.AlbumInfo
	for _, a := range albums {
		if ShouldDownloadAlbum(a.AvgPopularity, len(a.TopTrackNames)) {
			out = append(out, a)
		}
	}
	return out
}

func filterDownloaded(tracks []clients.LidarrTrack) map[string]bool {
	m := make(map[string]bool)
	for _, t := range tracks {
		if t.HasFile {
			m[strings.ToLower(strings.TrimSpace(t.Title))] = true
		}
	}
	return m
}

func matchAlbum(name string, lidarrAlbums []clients.LidarrAlbum) *clients.LidarrAlbum {
	norm := func(s string) string {
		return strings.ToLower(regexp.MustCompile(`[^\\w\\s]`).ReplaceAllString(strings.TrimSpace(s), ""))
	}
	target := norm(name)
	for i := range lidarrAlbums {
		if norm(lidarrAlbums[i].Title) == target {
			return &lidarrAlbums[i]
		}
	}
	for i := range lidarrAlbums {
		la := norm(lidarrAlbums[i].Title)
		if strings.Contains(la, target) || strings.Contains(target, la) {
			return &lidarrAlbums[i]
		}
	}
	return nil
}

func ensureLidarrArtist(a *store.Artist, lidarr *clients.LidarrClient) (*clients.LidarrArtist, error) {
	folder := a.RootFolder
	if folder == "" {
		if v, _ := store.SettingGet("default_root_folder"); v != "" {
			folder = v
		}
	}

	res, err := lidarr.AddArtist(a.Name, folder, 0)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res, nil
}

func waitForLidarrAlbums(lidarr *clients.LidarrClient, id int64, out *[]clients.LidarrAlbum) {
	for i := 0; i < 3; i++ {
		albums, err := lidarr.GetArtistAlbums(id)
		if err == nil && len(albums) > 0 {
			*out = albums
			return
		}
		log.Printf("Waiting for Lidarr albums (attempt %d/3)...", i+1)
	}
}