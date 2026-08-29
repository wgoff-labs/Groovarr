package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wgoff-labs/Groovarr/backend/internal/connections"
	"github.com/wgoff-labs/Groovarr/backend/internal/store"
)

// ArtistManageHandler handles GET /api/artist/:artistId/manage
func ArtistManageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get artist ID from URL: /api/artist/{artistId}/manage
	path := r.URL.Path
	// Expected format: /api/artist/{artistId}/manage
	const prefix = "/api/artist/"
	const suffix = "/manage"
	if !hasPrefix(path, prefix) || !hasSuffix(path, suffix) {
		http.Error(w, "Invalid artist manage path", http.StatusBadRequest)
		return
	}
	artistIDStr := path[len(prefix) : len(path)-len(suffix)]
	artistID, err := strconv.ParseInt(artistIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid artist ID", http.StatusBadRequest)
		return
	}

	// Get artist from store
	artist, err := store.ArtistGetByID(artistID)
	if err != nil {
		http.Error(w, "Artist not found", http.StatusNotFound)
		return
	}

	// Get connection manager to access Lidarr client
	mgr := connections.GetManager()
	lidarrClient := mgr.GetLidarrClient()
	if lidarrClient == nil {
		http.Error(w, "Lidarr not configured", http.StatusServiceUnavailable)
		return
	}

	// Fetch albums from Lidarr
	albums, err := lidarrClient.GetArtistAlbums(artist.LidarrID)
	if err != nil {
		http.Error(w, "Failed to fetch albums from Lidarr", http.StatusInternalServerError)
		return
	}

	// Get monitored tracks if download mode is tracks
	var monitoredTracks []MonitoredTrackResponse
	downloadMode, _ := store.SettingGet("general_download_mode")
	if downloadMode == "tracks" {
		tracks, err := lidarrClient.GetArtistTracks(artist.LidarrID)
		if err != nil {
			http.Error(w, "Failed to fetch tracks from Lidarr", http.StatusInternalServerError)
			return
		}

		// Get track preferences from store
		trackPrefs, err := store.GetTrackPreferencesForArtist(artistID)
		if err != nil {
			http.Error(w, "Failed to get track preferences", http.StatusInternalServerError)
			return
		}

		// Build monitored tracks response
		for _, track := range tracks {
			var currentScore *int
			// TODO: Get score from popularity cache
			// For now, we'll leave it nil

			monitoredTracks = append(monitoredTracks, MonitoredTrackResponse{
				LidarrTrackID: track.ID,
				Title:         track.Title,
				AlbumTitle:    track.Album.Title,
				TrackNumber:   track.TrackNumber,
				DiscNumber:    track.DiscNumber,
				CurrentScore:  currentScore,
				TrackState:    trackPrefs[track.ID],
			})
		}
	}

	// Build albums response
	var albumResponses []AlbumResponse
	for _, album := range albums {
		albumResponses = append(albumResponses, AlbumResponse{
			LidarrAlbumID: album.ID,
			Title:         album.Title,
			Year:          album.Year,
			TrackCount:    len(album.Tracks),
		})
	}

	response := ArtistManageResponse{
		Artist: ArtistResponse{
			ID:       artist.ID,
			Name:     artist.Name,
			LidarrID: artist.LidarrID,
		},
		Albums:     albumResponses,
		MonitoredTracks: monitoredTracks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Response structs
type ArtistManageResponse struct {
	Artist         ArtistResponse      `json:"artist"`
	Albums         []AlbumResponse     `json:"albums"`
	MonitoredTracks []MonitoredTrackResponse `json:"monitoredTracks"`
}

type ArtistResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	LidarrID int64  `json:"lidarrId"`
}

type AlbumResponse struct {
	LidarrAlbumID int64 `json:"lidarrAlbumId"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	TrackCount    int    `json:"trackCount"`
}

type MonitoredTrackResponse struct {
	LidarrTrackID int64   `json:"lidarrTrackId"`
	Title         string  `json:"title"`
	AlbumTitle    string  `json:"albumTitle"`
	TrackNumber   int     `json:"trackNumber"`
	DiscNumber    int     `json:"discNumber"`
	CurrentScore  *int    `json:"currentScore,omitempty"`
	TrackState    *string `json:"trackState,omitempty"`
}

// hasPrefix and hasSuffix are simple helper functions.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}