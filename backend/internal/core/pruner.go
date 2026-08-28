package core

import (
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
	PrunedTracks  int    `json:"pruned_tracks"`
	AlreadyPruned bool   `json:"already_pruned"`
	Error         string `json:"error,omitempty"`
}

// PruneDownloadedAlbums checks all downloaded albums and prunes below-threshold tracks.
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
		// Skip non-tracks mode artists
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

		deezerID := ""
		if artist.DeezerID != nil {
			deezerID = *artist.DeezerID
		}

		scores := GetArtistTrackScores(artist.Name, deezerID)

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
				// Check never-prune
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

				score := ScoreTrack(t.Title, scores, t.ID)
				if score >= cfg.PopularityThreshold {
					keep = append(keep, t)
				} else {
					prune = append(prune, t)
				}
			}

			if len(prune) == 0 || len(keep) == 0 {
				if len(keep) > 0 {
					store.SettingSet(prunedKey, "all_popular")
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
			store.SettingSet(prunedKey, "kept:"+strconv.Itoa(len(keep))+"_pruned:"+strconv.Itoa(deleted))

			results = append(results, PruneResult{
				ArtistName:   artist.Name,
				AlbumName:    la.Title,
				TotalTracks:  len(downloaded),
				KeptTracks:  len(keep),
				PrunedTracks: deleted,
			})
		}
	}

	return results, nil
}

// PruneSingleAlbum prunes below-threshold tracks from one specific album.
func PruneSingleAlbum(artistID int64, albumName string, lidarrAlbumID int64) *PruneResult {
	artist, err := store.ArtistGetByID(artistID)
	if err != nil || artist == nil {
		return &PruneResult{ArtistName: "unknown", AlbumName: albumName, Error: "artist not found"}
	}

	cfg := config.Load()
	deezerID := ""
	if artist.DeezerID != nil {
		deezerID = *artist.DeezerID
	}
	scores := GetArtistTrackScores(artist.Name, deezerID)

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
		score := ScoreTrack(t.Title, scores, t.ID)
		if score >= cfg.PopularityThreshold {
			keep = append(keep, t)
		} else {
			prune = append(prune, t)
		}
	}

	if len(prune) == 0 {
		lidarrID := lidarrAlbumID
		store.AlbumStatusSet(artist.ID, albumName, "pruned", &lidarrID)
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
	lidarrID := lidarrAlbumID
	store.AlbumStatusSet(artist.ID, albumName, "pruned", &lidarrID)

	log.Printf("Pruned %d/%d tracks from '%s' by %s", deleted, len(prune), albumName, artist.Name)
	return &PruneResult{
		ArtistName:   artist.Name,
		AlbumName:    albumName,
		TotalTracks:  len(downloaded),
		KeptTracks:   len(keep),
		PrunedTracks: deleted,
	}
}

// CheckDownloads finds pending albums that have finished downloading and auto-prunes them.
func CheckDownloads() ([]PruneResult, error) {
	albums, err := store.PendingAlbums()
	if err != nil || len(albums) == 0 {
		return nil, nil
	}

	lidarr, err := clients.NewLidarrClient()
	if err != nil {
		return nil, err
	}

	var results []PruneResult
	for _, a := range albums {
		lidarrAlbumID, ok := a["lidarr_album_id"].(int64)
		if !ok || lidarrAlbumID == 0 {
			continue
		}
		artistID, ok := a["artist_id"].(int64)
		if !ok {
			continue
		}
		albumName, _ := a["album_name"].(string)

		tracks, err := lidarr.GetAlbumTracks(lidarrAlbumID)
		if err != nil {
			continue
		}
		downloaded := filterDownloadedTracks(tracks)
		if len(downloaded) == 0 {
			continue
		}

		lidarrID := lidarrAlbumID
		store.AlbumStatusSet(artistID, albumName, "downloaded", &lidarrID)
		r := PruneSingleAlbum(artistID, albumName, lidarrAlbumID)
		if r != nil {
			results = append(results, *r)
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
