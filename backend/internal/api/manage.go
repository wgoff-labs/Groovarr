package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/connections"
	"github.com/groovarr/groovarr/backend/internal/store"
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
	if err != nil || artist == nil {
		http.Error(w, "Artist not found", http.StatusNotFound)
		return
	}

	// Get connection manager to access Lidarr client
	mgr := connections.GetManager()
	lidarrClient, err := mgr.GetLidarrClient()
	if err != nil {
		if WriteLidarrUnavailable(w, mgr) {
			return
		}
		http.Error(w, "Lidarr not configured: "+config.SanitizeError(err.Error()), http.StatusServiceUnavailable)
		return
	}

	// Fetch albums from Lidarr
	albums, err := lidarrClient.GetArtistAlbums(derefPtr(artist.LidarrID))
	if err != nil {
		http.Error(w, "Failed to fetch albums from Lidarr: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
		return
	}

	// Get track preferences map for all tracks
		trackPrefs, _ := store.GetTrackPreferences(artistID)
	
		// Get cached popularity scores for this artist
		popularityMap := make(map[int64]int)
		if pops, err := store.GetTrackPopularity(artistID); err == nil {
			for _, p := range pops {
				popularityMap[p.LidarrTrackID] = p.PlayCount
			}
		}

	// Build albums response (and collect track counts)
	albumResponses := make([]AlbumResponse, 0, len(albums))
	allTracks := make([]MonitoredTrackResponse, 0)

	for _, album := range albums {
		// Fetch track count per album
		albumTracks, _ := lidarrClient.GetAlbumTracks(album.ID)
		albumResponses = append(albumResponses, AlbumResponse{
			LidarrAlbumID: album.ID,
			Title:         album.Title,
			Year:          album.ReleaseYear(),
			Monitored:     album.Monitored,
			TrackCount:    len(albumTracks),
		})

		// Only build track list if download mode is tracks
		downloadMode, _ := store.SettingGet("download_mode")
		if downloadMode == "" {
			downloadMode = config.Get().DownloadMode
		}
		if downloadMode == "tracks" {
			for _, track := range albumTracks {
				// State is a plain string (empty = auto/no preference) so the
				// frontend can compare it with === against 'keep'/'hit'/'not_keep'/''.
				// A pointer would serialize as either null or be omitted entirely,
				// both of which break the React button-active comparisons.
				trackState := ""
				if s, ok := trackPrefs[track.ID]; ok {
					trackState = s
				}
				allTracks = append(allTracks, MonitoredTrackResponse{
					LidarrTrackID: track.ID,
					Title:         track.Title,
					AlbumTitle:    album.Title,
					AlbumLidarrID: album.ID,
					TrackNumber:   clients.TrackNumberInt(track),
					DiscNumber:    track.DiscNumber,
					Duration:      track.Duration,
					Downloaded:    track.HasFile,
					CurrentScore:  func() *int { if s, ok := popularityMap[track.ID]; ok { return &s }; return nil }(),
					State:         trackState,
				})
			}
		}
	}

	response := ArtistManageResponse{
		Artist: ArtistResponse{
			ID:         artist.ID,
			Name:       artist.Name,
			LidarrID:   derefPtr(artist.LidarrID),
			RootFolder: artist.RootFolder,
		},
		Albums: albumResponses,
		Tracks: allTracks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Response structs
type ArtistManageResponse struct {
	Artist ArtistResponse           `json:"artist"`
	Albums []AlbumResponse          `json:"albums"`
	Tracks []MonitoredTrackResponse `json:"tracks"`
}

type ArtistResponse struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	LidarrID   int64  `json:"lidarrId"`
	RootFolder string `json:"rootFolder"`
}

type AlbumResponse struct {
	LidarrAlbumID int64  `json:"lidarrId"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	TrackCount    int    `json:"trackCount"`
	Monitored     bool   `json:"monitored"`
}

type MonitoredTrackResponse struct {
	LidarrTrackID int64   `json:"lidarrId"`
	Title         string  `json:"title"`
	AlbumTitle    string  `json:"albumTitle"`
	AlbumLidarrID int64   `json:"albumLidarrId"`
	TrackNumber   int     `json:"trackNumber"`
	DiscNumber    int     `json:"discNumber"`
	Duration      int     `json:"duration"`
	Downloaded    bool    `json:"downloaded"`
	CurrentScore  *int    `json:"currentScore,omitempty"`
	// State is a plain string so it always serializes (empty = no preference set / auto).
	State string `json:"state"`
}

// hasPrefix and hasSuffix are simple helper functions.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// derefPtr safely dereferences an int64 pointer, returning 0 if nil.
func derefPtr(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
