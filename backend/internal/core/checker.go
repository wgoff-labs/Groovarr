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

	cfg := config.Load()
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
		deezerID := artist.DeezerID
		if deezerID == nil || *deezerID == "" {
			found, err := deezer.SearchArtist(artist.Name)
			if err != nil || found == nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Not found on Deezer: %v", err))
				results = append(results, result)
				continue
			}
			idStr := fmt.Sprintf("%d", found.ID)
			deezerID = &idStr
			store.ArtistUpdateDeezerID(artist.Name, idStr)
		}

		// Get releases
		var albums []clients.AlbumInfo
		var err error
		if fullScan {
			albums, err = deezer.GetTopAlbums(*deezerID)
		} else {
			albums, err = deezer.GetNewReleases(*deezerID, 90)
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
				// Wait for Lidarr to populate
				waitForLidarrAlbums(lidarr, *lidarrArtistID, &lidarrAlbums)
			}
		}

		for _, album := range popular {
			matched := matchAlbum(album.Name, lidarrAlbums)
			if matched == nil {
				store.CheckLogInsert(artist.ID, album.Name, &album.DeezerURL, album.AvgPopularity, false)
				result.SkippedAlbums = append(result.SkippedAlbums, fmt.Sprintf("%s (not in Lidarr)", album.Name))
				continue
			}

			// Check which tracks are already downloaded
			tracks, _ := lidarr.GetAlbumTracks(matched.ID)
			downloaded := filterDownloaded(tracks)

			popularNames := make(map[string]bool)
			for _, tp := range album.TrackPopularities {
				if tp.Popularity >= cfg.PopularityThreshold {
					popularNames[strings.ToLower(strings.TrimSpace(tp.Name))] = true
				}
			}

			alreadyHave := 0
			missing := 0
			for name := range popularNames {
				if downloaded[strings.ToLower(strings.TrimSpace(name))] {
					alreadyHave++
				} else {
					missing++
				}
			}

			if len(popularNames) > 0 && missing == 0 {
				store.AlbumStatusSet(artist.ID, album.Name, "pruned", &matched.ID)
				result.SkippedAlbums = append(result.SkippedAlbums, fmt.Sprintf("%s (all %d hits already downloaded)", album.Name, alreadyHave))
				continue
			}

			if err := lidarr.MonitorAndSearchAlbum(matched.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to search %s: %v", album.Name, err))
				continue
			}

			status := "downloaded"
			if missing > 0 {
				status = "pending"
			}
			store.AlbumStatusSet(artist.ID, album.Name, status, &matched.ID)
			store.CheckLogInsert(artist.ID, album.Name, &album.DeezerURL, album.AvgPopularity, true)

			if missing > 0 {
				result.AddedAlbums = append(result.AddedAlbums, fmt.Sprintf("%s — %d track(s) queued (%d already have)", album.Name, missing, alreadyHave))
			} else {
				result.AddedAlbums = append(result.AddedAlbums, fmt.Sprintf("%s (avg pop: %.1f, type: %s)", album.Name, album.AvgPopularity, album.AlbumType))
			}
			result.AlbumsAdded++
		}

		store.ArtistMarkChecked(artist.ID)
		results = append(results, result)
	}

	return results, nil
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
	if folder == nil || *folder == "" {
		if v, _ := store.SettingGet("default_root_folder"); v != "" {
			folder = &v
		}
	}
	fFolder := ""
	if folder != nil {
		fFolder = *folder
	}

	res, err := lidarr.AddArtist(a.Name, fFolder, 0)
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
