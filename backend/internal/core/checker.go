package core

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// checkLock serializes all /api/check calls so only one check runs at a time.
// The mutex is held for the duration of the entire check — multiple artists,
// Last.fm fetches, Lidarr searches, and all downloads.
var (
	checkLock    sync.Mutex
	checkRunning bool
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

// CheckRunning returns true if a check is currently in progress.
func CheckRunning() bool {
	checkLock.Lock()
	defer checkLock.Unlock()
	return checkRunning
}

// RunDailyCheck runs the daily popularity check for all (or one) watched artists.
// If a check is already running, this call blocks until it finishes unless
// force == "kill" (which returns an error so the caller can surface a warning).
func RunDailyCheck(artistFilter string, fullScan bool, force string) ([]CheckResult, error) {
	// Try to acquire the lock. If another check is running, block unless forced.
	if checkRunning {
		if force == "kill" {
			return nil, fmt.Errorf("cannot start new check: one is already running; use force='kill' only on the runner that started it")
		}
		// Wait until checkRunning becomes false (the holder releases the lock).
		for checkRunning {
			time.Sleep(100 * time.Millisecond)
		}
	}

	checkLock.Lock()
	// Mark that we're now the one running the check.
	checkRunning = true
	// Defer unlock and reset checkRunning so other calls can proceed when we exit.
	defer func() {
		checkLock.Unlock()
		checkRunning = false
	}()

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
	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return nil, fmt.Errorf("lidarr: %w", err)
	}

	var results []CheckResult

	for _, artist := range artists {
		result := CheckResult{ArtistName: artist.Name}
		log.Printf("Checking artist: %s", artist.Name)

		// Ensure artist exists in Lidarr first.
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

		// Fetch Lidarr albums (wait if Lidarr hasn't finished scanning yet).
		lidarrAlbums, _ := lidarr.GetArtistAlbums(*lidarrArtistID)
		if len(lidarrAlbums) == 0 {
			log.Printf("No Lidarr albums for %s, waiting for scan...", artist.Name)
			waitForLidarrAlbums(lidarr, *lidarrArtistID, &lidarrAlbums)
		}

		// Fetch Last.fm popularity data (primary, only source).
		lastfmScores := GetArtistTrackScores(artist.ID, artist.Name, "")
		log.Printf("[check] %s | lastfm scores: %d unique tracks", artist.Name, len(lastfmScores.NameScores))

		// Determine download mode.
		downloadMode := cfg.DownloadMode
		if artistMode, err := store.SettingGet("mode_" + artist.Name); err == nil && artistMode != "" {
			downloadMode = artistMode
		}

		// Score each Lidarr album by average Last.fm track score.
		// An album is "popular" if its average track score >= threshold.
		threshold := cfg.PopularityThreshold
		popular := make([]albumWithScore, 0, len(lidarrAlbums))
		for _, album := range lidarrAlbums {
			tracks, err := lidarr.GetAlbumTracks(album.ID)
			if err != nil || len(tracks) == 0 {
				continue
			}
			var total, scored int
			for _, t := range tracks {
				key := strings.ToLower(strings.TrimSpace(t.Title))
				// Score from Last.fm only; default 10 for truly missing tracks.
				var score int
				if s, ok := lastfmScores.NameScores[key]; ok {
					score = s
				} else {
					score = 10
				}
				total += score
				scored++
			}
			if scored == 0 {
				continue
			}
			avg := total / scored
			if avg >= threshold {
				popular = append(popular, albumWithScore{album: &album, avgScore: avg})
			}
		}
		result.NewAlbumsFound = len(popular)

		// Process popular albums.
		for _, aws := range popular {
			album := aws.album
			if downloadMode == "tracks" {
				// Per-track 3-state model.
				hitsKept, hitsFallen, pruned := processTracksThreeState(
					artist, album, lidarr, cfg.PopularityThreshold, lastfmScores, &result,
				)
				result.HitsKept += hitsKept
				result.HitsFallen += hitsFallen
				result.TracksPruned += pruned
			} else {
				// Album mode: monitor + search the whole album.
				if err := lidarr.MonitorAndSearchAlbum(album.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to search %s: %v", album.Title, err))
				}
			}
		}
		store.ArtistMarkChecked(artist.ID)
		results = append(results, result)
	}
	return results, nil
}

type albumWithScore struct {
	album    *clients.LidarrAlbum
	avgScore int
}

// processTracksThreeState applies the per-track 3-state (keep / hit / not_keep) model
// for one Lidarr album. Returns counters for hits kept, hits fallen, and tracks pruned.
func processTracksThreeState(
	artist *store.Artist,
	album *clients.LidarrAlbum,
	lidarr *clients.LidarrClient,
	threshold int,
	lastfmScores TrackScores,
	result *CheckResult,
) (hitsKept, hitsFallen, pruned int) {
	tracks, err := lidarr.GetAlbumTracks(album.ID)
	if err != nil || len(tracks) == 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("No tracks for %s: %v", album.Title, err))
		return
	}

	for _, t := range tracks {
		key := strings.ToLower(strings.TrimSpace(t.Title))
		// Score from Last.fm only; default 10 for truly missing tracks.
		var score int
		if s, ok := lastfmScores.NameScores[key]; ok {
			score = s
		} else {
			score = 10
		}
		_ = store.UpsertTrackPopularity(artist.ID, t.ID, score, key)

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
						_ = store.PruningLogInsert(artist.ID, album.Title, "unmonitored", "", 1)
						pruned++
					}
				}
			}
		}
	}

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