package core

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// PruneResult is the result of pruning one album.
type PruneResult struct {
	ArtistName    string `json:"artist_name"`
	AlbumName     string `json:"album_name"`
	TotalTracks   int    `json:"total_tracks"`
	KeptTracks    int    `json:"kept_tracks"`
	PrunedTracks  int    `json:"pruned_truned"`
	AlreadyPruned bool   `json:"already_pruned"`
	Error         string `json:"error,omitempty"`
}

// PruneDownloadedAlbums checks all downloaded albums and prunes below-threshold tracks.
// Respects the 3-state model: "keep" and "hit" tracks are never auto-pruned.
func PruneDownloadedAlbums(artistFilter string, force bool) ([]PruneResult, error) {
	var artists []*store.Artist
	if artistFilter != "" {
		a, err := store.ArtistGet(artistFilter)
		if err != nil || a == nil {
			return nil, nil
		}
		artists = []*store.Artist{a}
	} else {
		var err error
		artists, err = store.ArtistList()
		if err != nil {
			return nil, err
		}
	}

	cfg := config.Load()
	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return nil, err
	}

	var results []PruneResult

	for _, artist := range artists {
		// Only prune in tracks mode.
		mode, _ := store.SettingGet("mode_" + artist.Name)
		if mode == "" {
			mode = cfg.DownloadMode
		}
		if mode != "tracks" {
			continue
		}

		if artist.LidarrID == nil || *artist.LidarrID == 0 {
			continue
		}

		lidarrAlbums, err := lidarr.GetArtistAlbums(*artist.LidarrID)
		if err != nil || len(lidarrAlbums) == 0 {
			continue
		}

		scores := GetArtistTrackScores(artist.ID, artist.Name, "")

		for _, la := range lidarrAlbums {
			prunedKey := "pruned_" + strconv.FormatInt(artist.ID, 10) + "_" + la.Title
			if !force {
				if v, _ := store.SettingGet(prunedKey); v != "" {
					continue
				}
			}

			tracks, err := lidarr.GetAlbumTracks(la.ID)
			if err != nil || len(tracks) == 0 {
				continue
			}

			downloaded := filterDownloadedTracks(tracks)
			if len(downloaded) == 0 {
				continue
			}

			var keep, prune []clients.LidarrTrack
			for _, t := range downloaded {
				// Check never-prune list.
				npTracks, _ := store.NeverPruneTracks(artist.ID, la.Title)
				isProtected := false
				for _, np := range npTracks {
					if strings.EqualFold(strings.TrimSpace(np), strings.TrimSpace(t.Title)) {
						isProtected = true
						break
					}
				}
				if isProtected {
					keep = append(keep, t)
					continue
				}

				// Check manual 3-state preference.
				state, _ := store.GetTrackPreference(artist.ID, t.ID)
				if state == "keep" || state == "hit" {
					// Never auto-prune explicit keep or hit.
					keep = append(keep, t)
					continue
				}

				// not_keep or no preference — use score threshold.
				score := ScoreTrack(t.Title, scores, t.ID)
				if score >= cfg.PopularityThreshold {
					keep = append(keep, t)
				} else {
					prune = append(prune, t)
				}
			}

			if len(prune) == 0 || len(keep) == 0 {
						if len(keep) > 0 {
							_ = store.SettingUpdate(prunedKey, "all_popular")
						}
						continue
					}

			deleted := 0
			for _, t := range prune {
				if t.TrackFileID != 0 {
					if err := lidarr.DeleteTrackFile(t.TrackFileID); err == nil {
						deleted++
					}
				}
			}

			lidarr.SetAlbumMonitored(la.ID, false)
					_ = store.SettingUpdate(prunedKey, "kept:"+strconv.Itoa(len(keep))+"_pruned:"+strconv.Itoa(deleted))

			results = append(results, PruneResult{
				ArtistName:   artist.Name,
				AlbumName:    la.Title,
				TotalTracks:  len(downloaded),
				KeptTracks:   len(keep),
				PrunedTracks: deleted,
			})
		}
	}

	return results, nil
}

// PruneSingleAlbum prunes below-threshold tracks from one specific album.
// Respects 3-state preferences: "keep" and "hit" tracks are never pruned.
func PruneSingleAlbum(artistID int64, albumName string, lidarrAlbumID int64) *PruneResult {
	artist, err := store.ArtistGetByID(artistID)
	if err != nil || artist == nil {
		return &PruneResult{ArtistName: "unknown", AlbumName: albumName, Error: "artist not found"}
	}

	cfg := config.Load()
	scores := GetArtistTrackScores(artist.ID, artist.Name, "")

	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return &PruneResult{ArtistName: artist.Name, AlbumName: albumName, Error: err.Error()}
	}

	tracks, err := lidarr.GetAlbumTracks(lidarrAlbumID)
	if err != nil || len(tracks) == 0 {
		return &PruneResult{ArtistName: artist.Name, AlbumName: albumName, Error: "no tracks found"}
	}

	downloaded := filterDownloadedTracks(tracks)
	if len(downloaded) == 0 {
		return &PruneResult{ArtistName: artist.Name, AlbumName: albumName, Error: "no downloaded files"}
	}

	neverPruneNames := make(map[string]bool)
	np, _ := store.NeverPruneTracks(artist.ID, albumName)
	for _, t := range np {
		neverPruneNames[strings.ToLower(strings.TrimSpace(t))] = true
	}

	var keep, prune []clients.LidarrTrack
	for _, t := range downloaded {
		name := strings.ToLower(strings.TrimSpace(t.Title))
		if neverPruneNames[name] {
			keep = append(keep, t)
			continue
		}

		state, _ := store.GetTrackPreference(artist.ID, t.ID)
		if state == "keep" || state == "hit" {
			keep = append(keep, t)
			continue
		}

		score := ScoreTrack(t.Title, scores, t.ID)
		if score >= cfg.PopularityThreshold {
			keep = append(keep, t)
		} else {
			prune = append(prune, t)
		}
	}

	if len(prune) == 0 {
		return &PruneResult{
			ArtistName:   artist.Name,
			AlbumName:    albumName,
			TotalTracks:  len(downloaded),
			KeptTracks:   len(keep),
			PrunedTracks: 0,
		}
	}

	if len(keep) == 0 {
		return &PruneResult{
			ArtistName:   artist.Name,
			AlbumName:    albumName,
			TotalTracks:  len(downloaded),
			KeptTracks:   0,
			PrunedTracks: 0,
			Error:        "All tracks below threshold — keeping album",
		}
	}

	deleted := 0
	for _, t := range prune {
		if t.TrackFileID != 0 {
			if err := lidarr.DeleteTrackFile(t.TrackFileID); err == nil {
				deleted++
			}
		}
	}

	lidarr.SetAlbumMonitored(lidarrAlbumID, false)
	// Note: AlbumStatusSet doesn't exist in store, skipping for now

	log.Printf("Pruned %d/%d tracks from '%s' by %s", deleted, len(prune), albumName, artist.Name)
	return &PruneResult{
		ArtistName:   artist.Name,
		AlbumName:    albumName,
		TotalTracks:  len(downloaded),
		KeptTracks:   len(keep),
		PrunedTracks: deleted,
	}
}

// CheckDownloads finds albums in Lidarr that have finished downloading and auto-prunes
// below-threshold tracks. It iterates over known artists, checks their Lidarr albums for
// downloaded tracks, and invokes the pruning logic for any album with completed downloads.
//
// The function:
//   1. Loads config and creates a Lidarr client via config.Load() (same as PruneDownloadedAlbums).
//   2. Retrieves all tracked artists from the store.
//   3. For each artist, fetches their Lidarr albums and identifies which have downloaded tracks.
//   4. Calls PruneSingleAlbum for any album that has finished downloading,
//      respecting the 3-state keep/hit/prune model and the DownloadMode setting.
//
// Return values:
//   - A slice of PruneResult, one per pruned album (may be empty if nothing to process).
//   - An error if the Lidarr client cannot be initialized or artist/album data cannot be fetched.
func CheckDownloads() ([]PruneResult, error) {
	cfg := config.Load()

	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Lidarr client: %w", err)
	}

	artists, artistErr := store.ArtistList()
	if artistErr != nil {
		return nil, fmt.Errorf("failed to fetch artists: %w", artistErr)
	}

	if len(artists) == 0 {
		return nil, nil
	}

	var results []PruneResult

	for _, artist := range artists {
		if artist.LidarrID == nil || *artist.LidarrID == 0 {
			continue
		}

		// Only prune in tracks mode.
		mode, _ := store.SettingGet("mode_" + artist.Name)
		if mode == "" {
			mode = cfg.DownloadMode
		}
		if mode != "tracks" {
			continue
		}

		lidarrAlbums, err := lidarr.GetArtistAlbums(*artist.LidarrID)
		if err != nil || len(lidarrAlbums) == 0 {
			continue
		}

		for _, la := range lidarrAlbums {
			tracks, err := lidarr.GetAlbumTracks(la.ID)
			if err != nil || len(tracks) == 0 {
				continue
			}

			downloaded := filterDownloadedTracks(tracks)
			if len(downloaded) == 0 {
				continue
			}

			// Run the single-album pruning logic
			pruneResult := PruneSingleAlbum(artist.ID, la.Title, la.ID)
			if pruneResult.Error == "" {
				results = append(results, *pruneResult)
			}
		}
	}

	return results, nil
}

func filterDownloadedTracks(tracks []clients.LidarrTrack) []clients.LidarrTrack {
	var out []clients.LidarrTrack
	for _, t := range tracks {
		if t.HasFile {
			out = append(out, t)
		}
	}
	return out
}